// MIT License
//
// Copyright (c) 2021 Yuchen Niu and EASE lab
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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

var (
	funcName = flag.String("func", "", "")
)

func main() {
	flag.Parse()
	var (
		vmStep   = 2
		maxVMs   = 32
		cpuID    = 31
		socket   = 1
		funcs    = *funcName
		dirs     = []string{"top", "fb", "fl", "br", "bb", "mb", "cb"}
		nodeList = []string{"+IPC,+ILP", "!+Frontend_Bound*/2,+MUX", "!+Fetch_Latency*/3,+MUX", "!+Branch_Resteers*/4,+MUX",
			"!+Backend_Bound*/2,+MUX", "!+Memory_Bound*/3,+MUX", "!+Core_Bound*/3,+MUX"}
	)

	// run
	for i, nodes := range nodeList {
		numactl := exec.Command("numactl", "--cpunodebind=0", "--")
		test := exec.Command("go", "test", "-v", "-timeout", "99999s", "-run", "TestProfileIncrementConfiguration", "-args", "-funcNames", funcs, "-vmIncrStep", strconv.Itoa(vmStep), "-maxVMNum", strconv.Itoa(maxVMs), "-bindSocket", strconv.Itoa(socket), "-profileCPUID", strconv.Itoa(cpuID), "-nodes", nodes, "-benchDirTest", dirs[i])
		numactl.Args = append(numactl.Args, test.Args...)
		numactl.Stdin = os.Stdin
		numactl.Stdout = os.Stdout
		numactl.Stderr = os.Stdout
		fmt.Println("Command: ", numactl)
		if err := numactl.Run(); err != nil {
			fmt.Println(err)
			return
		}
	}
}
