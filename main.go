package main

import (
	"gin_server/services"
	"github.com/gin-gonic/gin"
	"log"
)

func main() {
	r := gin.Default()

	// Define routes
	r.POST("/run-packager", services.RunShakaPackager)
	r.POST("/meta", services.GetShakaPackagerInfo)
	r.POST("/file-meta", services.GetFileInfo)
	r.POST("/convert-mp3-to-mp4", services.ConvertMP3ToMP4)
	r.POST("/convert-mkv-to-mp4", services.ConvertMKVToMP4)

	// Start the server
	log.Println("Server running on http://localhost:8080")
	err := r.Run(":8080")
	if err != nil {
		return
	}
}
