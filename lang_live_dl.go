package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
)

type memberData struct {
	Id           int    `json:"id"`
	Name         string `json:"name"`
	EnableNotify bool   `json:"enable_notify"`
	IconPath     string `json:"icon"`
}

const (
	folderPath     = "data"
	tempFolderPath = "temp"
)

func main() {
	fmt.Println("love kuri")
	readConfig()
	createFolders()

	members := readConfig()
	downloadTable := initDownloadTable(members)
	var downloadTableMtx sync.Mutex
	threadCount := 0

	for {
		for _, member := range members {
			startDownloadThraed(member, &downloadTableMtx, downloadTable, &threadCount)
		}
		if threadCount > len(members) {
			fmt.Printf("current thread cnt = %v\n", threadCount)
		}
		time.Sleep(2 * time.Second)
	}
}

func readConfig() []memberData {
	data, err := os.ReadFile("config.json")
	if err != nil {
		panic(err)
	}
	memberData := []memberData{}
	if err := json.Unmarshal(data, &memberData); err != nil {
		panic(err)
	}
	return memberData
}

func createFolders() {
	if err := os.MkdirAll(folderPath, os.ModePerm); err != nil {
		panic(fmt.Sprintf("create folder error, path = %v, err = %v\n", folderPath, err))
	}
	if err := os.MkdirAll(tempFolderPath, os.ModePerm); err != nil {
		panic(fmt.Sprintf("create folder error, path = %v, err = %v\n", folderPath, err))
	}
}

func initDownloadTable(members []memberData) map[int]bool {
	downloadTable := map[int]bool{}
	for _, member := range members {
		downloadTable[member.Id] = false
	}
	return downloadTable
}

func startDownloadThraed(member memberData, downloadTableMtx *sync.Mutex, downloadTable map[int]bool, threadCount *int) {
	downloadTableMtx.Lock()
	defer downloadTableMtx.Unlock()
	if !downloadTable[member.Id] {
		downloadTable[member.Id] = true
		*threadCount += 1
		go func(member memberData) {
			downloadForMember(&member)
			downloadTableMtx.Lock()
			defer downloadTableMtx.Unlock()
			downloadTable[member.Id] = false
			*threadCount -= 1
		}(member)
	}
}

func downloadForMember(member *memberData) {
	filename := fmt.Sprintf("%v.%v", member.Name, time.Now().Format("2006.01.02 15.04.05"))
	tempOutfilePath := filepath.Join(tempFolderPath, fmt.Sprintf("%v.flv", filename))
	if !downloadVideo(member, tempOutfilePath) {
		return
	}
	outfilePath := filepath.Join(folderPath, fmt.Sprintf("%v.mp4", filename))
	if err := flvToMp4(tempOutfilePath, outfilePath); err != nil {
		fmt.Printf("flv to mp4 failed, err = %v\n", err)
		return
	}
	fmt.Printf("download completed, name = %v\n", member.Name)
}

func downloadVideo(member *memberData, outfilePath string) bool {
	url := fmt.Sprintf("https://video-ws-hls-aws.lv-play.com/live/%vY.flv", member.Id)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("http get failed, name = %v, err = %v\n", member.Name, err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return false
	} else if resp.StatusCode != 200 {
		fmt.Printf("status = %v\n", resp.Status)
		return false
	}
	return writeRespToFile(member, outfilePath, resp)
}

func writeRespToFile(member *memberData, outfilePath string, resp *http.Response) bool {
	outfile, err := os.Create(outfilePath)
	if err != nil {
		fmt.Printf("create file error, path = %v, err = %v\n", outfilePath, err)
		return false
	}
	defer outfile.Close()

	if member.EnableNotify {
		if err := beeep.Notify("stream start", member.Name, fmt.Sprintf("assets/%v.jpg", member.Name)); err != nil {
			fmt.Printf("Notify error = %v", err)
		}
	}
	n, err := io.Copy(outfile, resp.Body)
	if err != nil {
		fmt.Printf("download failed, name = %v, err = %v\n", member.Name, err)
	}
	return n > 0
}

func flvToMp4(fromPath, toPath string) error {
	if err := exec.Command("ffmpeg", "-i", fromPath, "-codec", "copy", toPath).Run(); err != nil {
		return err
	}
	return os.Remove(fromPath)
}
