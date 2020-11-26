// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov and EASE lab
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	pb "github.com/ustiugov/fccd-orchestrator/helloworld"
	"google.golang.org/grpc"
)

func main() {
	urlFile := flag.String("urlf", "urls.csv", "File with functions' URLs")
	rps := flag.Int("rps", 1, "Target requests per second")
	runTime := flag.Int("time", 5, "Run the benchmark for X seconds")

	flag.Parse()

	log.Infof("Reading the URLs from the file: %s", *urlFile)

	urls, err := readLines(*urlFile)
	if err != nil {
		log.Fatal("Failed to read the URL files:", err)
	}

	runBenchmark(*rps, *runTime, urls)
}

func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func runBenchmark(rps, runTime int, urls []string) {
	timeout := time.After(time.Duration(runTime) * time.Second)
	tick := time.Tick(time.Duration(1000/rps) * time.Millisecond)

	var issued int

	for {
		select {
		case <-timeout:
			log.Println("Benchmark finished!")
			return
		case <-tick:
			url := urls[issued%len(urls)]
			go invokeFunction(url)

			issued++
		}
	}
}

func invokeFunction(url string) {
	address := fmt.Sprintf("%s:%d", url, 8080)

	conn, err := grpc.Dial(address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := pb.NewGreeterClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, err = c.SayHello(ctx, &pb.HelloRequest{Name: "faas"})
	if err != nil {
		log.Warn("Failed to invoke ", address)
	}

	return
}
