package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

type ffprobeOutput struct {
	Streams []struct {
		Width  int `json:"width,omitempty"`
		Height int `json:"height,omitempty"`
	} `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		filePath,
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error running ffprobe: %w", err)
	}

	var output ffprobeOutput
	err = json.Unmarshal(buf.Bytes(), &output)
	if err != nil {
		return "", fmt.Errorf("error unmarshiling buffer")
	}

	if len(output.Streams) == 0 {
		return "", fmt.Errorf("output string is empty")
	}
	width := output.Streams[0].Width
	height := output.Streams[0].Height

	if width == 16*height/9 {
		return "16:9", nil
	} else if height == 16*width/9 {
		return "9:16", nil
	}
	return "other", nil
}

func processVideoForFastStart(filePath string) (string, error) {
	outputFilePath := filePath + ".processing"
	cmd := exec.Command(
		"ffmpeg",
		"-i", filePath,
		"-c", "copy",
		"-movflags",
		"faststart",
		"-f", "mp4",
		outputFilePath,
	)

	var stdOut bytes.Buffer
	cmd.Stdout = &stdOut
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error running ffmpeg: %w", err)
	}

	return outputFilePath, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	bucket, key, err := splitVideoBucketKey(*video.VideoURL)
	if err != nil {
		return database.Video{}, fmt.Errorf("error spliting video bucket key: %v", err)
	}

	presignedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, 15*time.Minute)
	if err != nil {
		return database.Video{}, fmt.Errorf("error generating presigned URL: %v", err)
	}
	video.VideoURL = &presignedURL

	return video, nil
}

func splitVideoBucketKey(bucketAndKey string) (string, string, error) {
	splitedData := strings.Split(bucketAndKey, ",")
	if len(splitedData) != 2 {
		return "", "", fmt.Errorf("error bucket and key string unformated")
	}

	return splitedData[0], splitedData[1], nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	s3PreSignedClient := s3.NewPresignClient(s3Client)
	presignObject, err := s3PreSignedClient.PresignGetObject(context.Background(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", fmt.Errorf("failed creating presign object: %v", err)
	}

	return presignObject.URL, nil
}
