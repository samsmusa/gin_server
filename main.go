package main

import (
	"gin_server/services"
	"github.com/gin-gonic/gin"
	"log"
)

func main() {
	r := gin.Default()

	r.POST("/run-packager", services.RunShakaPackager)
	r.POST("/meta", services.GetShakaPackagerInfo)
	r.POST("/file-meta", func(c *gin.Context) {
		file, _ := services.SingleFileHandler(c)
		services.GetFileInfo(c, file)
	})
	r.POST("/convert-mp3-to-mp4", func(c *gin.Context) {
		file, _ := services.SingleFileHandler(c)
		services.ConvertMP3ToMP4(c, file)
	})
	r.POST("/convert-mkv-to-mp4", func(c *gin.Context) {
		file, _ := services.SingleFileHandler(c)
		services.ConvertMKVToMP4(c, file)
	})
	log.Println("Server running on http://localhost:8080")
	err := r.Run(":8080")
	if err != nil {
		return
	}
}
