package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

func main() {
	funcPath := flag.String("funcPath", "./knative_workloads", "Path to the folder with *.yml files")
	funcJSONFile := flag.String("jsonFile", "./examples/deployer/functions.json", "Path to the JSON file with functions to deploy")
	urlFile := flag.String("urlFile", "urls.txt", "File with functions' URLs")

	flag.Parse()

	log.Debug("Function files are taken from ", *funcPath)

	funcSlice := getFuncSlice(*funcJSONFile)

	urls := deploy(*funcPath, funcSlice)

	writeURLs(*urlFile, urls)

	log.Infoln("Deployment finished")
}

// Functions is an object for unmarshalled JSON with functions to deploy.
type Functions struct {
	Functions []functionType `json:"functions"`
}

type functionType struct {
	File string `json:"file"`

	// number of functions to deploy from the same file (with different names)
	Count int `json:"count"`
}

func getFuncSlice(file string) []functionType {
	log.Debugf("Opening JSON file with functions: %s\n", file)
	jsonFile, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer jsonFile.Close()

	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Fatal(err)
	}

	var functions Functions

	json.Unmarshal(byteValue, &functions)

	for i := 0; i < len(functions.Functions); i++ {
		log.Infof("File name : %v\n", functions.Functions[i].File)
	}

	return functions.Functions
}

func deploy(funcPath string, funcSlice []functionType) []string {
	var urls []string
	sem := make(chan bool, 1) // limit the number of parallel deployments

	for k, fType := range funcSlice {
		for i := 0; i < fType.Count; i++ {

			sem <- true

			fID := fmt.Sprintf("f%d_%d", k, i)
			url := fmt.Sprintf("%s.%s", fID, "default.192.168.1.240.xip.io")
			urls = append(urls, url)

			filePath := filepath.Join(funcPath, fType.File)

			go func(fID, filePath string) {
				defer func() { <-sem }()

				deployFunction(fID, filePath)
			}(fID, filePath)
		}
	}

	for i := 0; i < cap(sem); i++ {
		sem <- true
	}

	return urls
}

func deployFunction(fID, filePath string) {
	cmd := exec.Command("kn", "service", "apply", fID, "-f", filePath)
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to deploy function %s, %s: %v\n", fID, filePath, err)
	}

	log.Info("Deployed function", fID)
}

func writeURLs(filePath string, urls []string) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		log.Fatalf("failed creating file: %s", err)
	}

	datawriter := bufio.NewWriter(file)

	for _, url := range urls {
		_, err := datawriter.WriteString(url + "\n")
		if err != nil {
			log.Fatal("Failed to write the URLs to a file ", err)
		}
	}

	datawriter.Flush()
	file.Close()
	return
}
