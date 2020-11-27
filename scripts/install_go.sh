#!/bin/bash

set -e

# GO
wget https://golang.org/dl/go1.14.6.linux-amd64.tar.gz

sudo tar -C /usr/local -xzf go1.14.6.linux-amd64.tar.gz

export PATH=$PATH:/usr/local/go/bin

sudo sh -c  "echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile"

go env -w GOPRIVATE=github.com/ustiugov/*
