package main

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	const maxMemory = 10 << 20
	_ = r.ParseMultipartForm(maxMemory)

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "unable to extract form file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if mediaType != "image/jpg" && mediaType != "image/png" {
		respondWithError(w, http.StatusInternalServerError, "content-type not allowed", err)
		return
	}
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type for thumbnail", nil)
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

	randomID := make([]byte, 32)
	_, err = rand.Read(randomID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed creating random name", err)
		return
	}
	encoded := base64.RawURLEncoding.EncodeToString(randomID)

	assetPath := getAssetPath(encoded, mediaType)
	filePath := cfg.getAssetDiskPath(assetPath)

	newFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed creating a new file.", err)
		return
	}

	if _, err := io.Copy(newFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed loading thumbnail on server", err)
		return
	}

	// url := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", cfg.port, videoID)
	//
	// dataBase64 := base64.StdEncoding.EncodeToString(data)
	// url := fmt.Sprintf("data:%s;base64,%s", mediaType, dataBase64)
	url := cfg.getAssetURL(assetPath)
	video.ThumbnailURL = &url

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error uploading video to database", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
