package main

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
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

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error getting video from database", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
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

	tempVideo, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, 500, "error creating temporary video", err)
		return
	}
	defer os.Remove(tempVideo.Name())
	defer tempVideo.Close()

	_, err = io.Copy(tempVideo, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error copyng video to temporary file", err)
		return
	}

	_, err = tempVideo.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed reseting file pointer", err)
		return
	}

	randomID := make([]byte, 32)
	_, err = rand.Read(randomID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed creating random name", err)
		return
	}
	encoded := base64.RawURLEncoding.EncodeToString(randomID)

	videoAspectRate, err := getVideoAspectRatio(tempVideo.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error stracting video rate", err)
	}
	if videoAspectRate == "16:9" {
		encoded = "landscape/" + encoded
	} else if videoAspectRate == "9:16" {
		encoded = "portrait/" + encoded
	} else {
		encoded = "other/" + encoded
	}

	assetPath := getAssetPath(encoded, mediaType)

	cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(assetPath),
		Body:        tempVideo,
		ContentType: aws.String(mediaType),
	})

	url := cfg.getS3VideoUrl(cfg.s3Bucket, cfg.s3Region, assetPath)
	video.VideoURL = &url
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error uploading video to database", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
