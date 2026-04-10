package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxMemory = 1 << 30
	_ = r.ParseMultipartForm(maxMemory)

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	video, err := cfg.authenticateAndGetVideo(r, videoID)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "error authenticating", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "unable to extract form file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusInternalServerError, "content-type not allowed", err)
		return
	}
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type for video", nil)
		return
	}

	tempVideo, err := saveUploadToTempFile(file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed saving upload to temp File", err)
		return
	}

	_, err = tempVideo.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed reseting file pointer", err)
		return
	}

	key, err := buildS3Key(tempVideo.Name(), mediaType)

	processedVideoPath, err := processVideoForFastStart(tempVideo.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error proccessing video for fast start", err)
		return
	}
	defer os.Remove(processedVideoPath)

	processedVideo, err := os.Open(processedVideoPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error reading processed file", err)
		return
	}
	defer processedVideo.Close()

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(key),
		Body:        processedVideo,
		ContentType: aws.String(mediaType),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error putting s3 object", err)
		return
	}

	url := cfg.getS3VideoUrl(cfg.s3Bucket, cfg.s3Region, key)
	video.VideoURL = &url
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error uploading video to database", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

func (cfg *apiConfig) authenticateAndGetVideo(r *http.Request, videoID uuid.UUID) (database.Video, error) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		return database.Video{}, fmt.Errorf("unauthorized: %w", err)
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		return database.Video{}, fmt.Errorf("invalid token: %w", err)
	}
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		return database.Video{}, fmt.Errorf("video not found: %w", err)
	}
	if video.UserID != userID {
		return database.Video{}, fmt.Errorf("not authorized")
	}
	return video, nil
}

func saveUploadToTempFile(file io.Reader) (*os.File, error) {
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(tempFile, file); err != nil {
		os.Remove(tempFile.Name())
		return nil, err
	}
	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		os.Remove(tempFile.Name())
		return nil, err
	}
	return tempFile, nil
}

func buildS3Key(tempFilePath, mediaType string) (string, error) {
	randomID := make([]byte, 32)
	_, err := rand.Read(randomID)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(randomID)

	videoAspectRate, err := getVideoAspectRatio(tempFilePath)
	if err != nil {
		return "", err
	}
	if videoAspectRate == "16:9" {
		encoded = "landscape/" + encoded
	} else if videoAspectRate == "9:16" {
		encoded = "portrait/" + encoded
	} else {
		encoded = "other/" + encoded
	}

	assetPath := getAssetPath(encoded, mediaType)
	return assetPath, nil
}
