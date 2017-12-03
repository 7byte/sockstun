package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	logging "github.com/op/go-logging"
)

type config struct {
	Server     string        `json:"server"`
	Nodes      []node        `json:"nodes"`
	UDPTimeout time.Duration `json:"udpTimeout"`
	LogLevel   logging.Level `json:"logLevel"`
}

type node struct {
	Port     int    `json:"port"`
	Cipher   string `json:"cipher"`
	Password string `json:"password"`
}

func newConfig(confPath string) (c *config, err error) {
	var (
		file *os.File
		blob []byte
	)
	c = new(config)
	if file, err = os.Open(confPath); err != nil {
		fmt.Printf("open conf error, err = %v\n", err)
		return
	}
	defer file.Close()

	if blob, err = ioutil.ReadAll(file); err != nil {
		fmt.Printf("read conf err, err = %v\n", err)
		return
	}
	if err = json.Unmarshal(blob, c); err != nil {
		fmt.Printf("Unmarshal conf err, err = %v\n", err)
		return
	}
	return
}
