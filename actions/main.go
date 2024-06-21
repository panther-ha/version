package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/types"
	"github.com/coreos/go-semver/semver"
	types2 "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	"io"
	"log"
	"os"
	"strings"
)

var username = flag.String("username", "", "aliyun docker login username")
var password = flag.String("password", "", "aliyun docker login password")

func main() {
	flag.Parse()
	data, err := os.ReadFile("../stable.json")
	if err != nil {
		log.Fatal(err)
		return
	}
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal(err)
		return
	}
	var configs map[string]interface{}
	if err = json.Unmarshal(data, &configs); err != nil {
		log.Fatal(err)
		return
	}
	if _, ok := configs["docker_mirror"]; !ok {
		log.Fatal(`Missing "docker_mirror" key in stable.json`)
		return
	}
	var mirrors = configs["docker_mirror"].(map[string]interface{})
	for repo, mirror := range mirrors {
		fmt.Println("repo:", repo, "mirror:", mirror.(string))
		var tag string
		tag, err = getDockerRepositoryLastTag(ctx, repo)
		if err != nil {
			log.Fatal(err)
			continue
		}
		err = pullTagPush(ctx, cli, repo, mirror.(string), tag)
		if err != nil {
			log.Fatal(err)
			continue
		}
	}
}

func pullTagPush(ctx context.Context, cli *client.Client, repo, mirror, tag string) error {
	imageSource := fmt.Sprintf("%s:%s", repo, tag)
	imageTarget := fmt.Sprintf("%s:%s", mirror, tag)
	log.Println("Pulling", imageSource)
	reader, err := cli.ImagePull(ctx, imageSource,
		types2.ImagePullOptions{Platform: "linux/arm64"})
	if err != nil {
		return err
	}
	defer reader.Close()

	// Wait for the pull to complete
	_, err = io.Copy(os.Stdout, reader)
	if err != nil {
		return err
	}

	// Tag the image
	err = cli.ImageTag(ctx, imageSource, imageTarget)
	if err != nil {
		return err
	}
	log.Println("Pushing", imageTarget)
	authConfig := registry.AuthConfig{
		Username: *username,
		Password: *password,
	}
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		return err
	}
	authStr := base64.URLEncoding.EncodeToString(encodedJSON)

	// Push the image
	pushReader, err := cli.ImagePush(ctx, imageTarget,
		types2.ImagePushOptions{
			RegistryAuth: authStr,
			Platform:     "linux/arm64",
		})
	if err != nil {
		return err
	}
	defer pushReader.Close()

	// Wait for the push to complete
	_, err = io.Copy(os.Stdout, pushReader)
	if err != nil {
		return err
	}
	return nil
}

func getDockerRepositoryLastTag(ctx context.Context, repo string) (string, error) {
	sys := &types.SystemContext{}
	url := fmt.Sprintf(`docker://%s`, repo)
	imgRef, err := parseDockerRepositoryReference(url)
	if err != nil {
		return "", err
	}
	result, err := docker.GetRepositoryTags(ctx, sys, imgRef)
	if err != nil {
		return "", err
	}

	var tags []*semver.Version
	var originalTags = make(map[string]string)
	for _, tag := range result {
		if strings.HasPrefix(tag, "sha256") {
			continue
		}
		sps := strings.Split(tag, ".")
		spl := len(sps)
		if spl >= 2 && spl <= 3 {
			if v, err := semver.NewVersion(strings.Trim(tag, "vV")); err == nil {
				tags = append(tags, v)
				originalTags[v.String()] = tag
			}
		}
	}
	if len(tags) == 0 {
		return "", errors.New("no valid tags found")
	}
	semver.Sort(tags)
	originalTag, ok := originalTags[tags[len(tags)-1].String()]
	if !ok {
		return "", errors.New("no valid tags found")
	}
	return originalTag, nil
}

func parseDockerRepositoryReference(refString string) (types.ImageReference, error) {
	if !strings.HasPrefix(refString, docker.Transport.Name()+"://") {
		return nil, errors.Errorf("docker: image reference %s does not start with %s://", refString, docker.Transport.Name())
	}

	parts := strings.SplitN(refString, ":", 2)
	if len(parts) != 2 {
		return nil, errors.Errorf(`Invalid image name "%s", expected colon-separated transport:reference`, refString)
	}

	ref, err := reference.ParseNormalizedNamed(strings.TrimPrefix(parts[1], "//"))
	if err != nil {
		return nil, err
	}

	if !reference.IsNameOnly(ref) {
		return nil, errors.New(`No tag or digest allowed in reference`)
	}

	// Checks ok, now return a reference. This is a hack because the tag listing code expects a full image reference even though the tag is ignored
	return docker.NewReference(reference.TagNameOnly(ref))
}
