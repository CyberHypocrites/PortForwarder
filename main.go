package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//More global values
var Rules CSafeRule
var ConfigFileName = "rules.json"
var SimultaneousConnections CSafeConnections
var Verbose = 1
var SaveBeforeExit = true

const Version = "1.4.0 / Build 11"

type CSafeConnections struct {
	SimultaneousConnections []int
	mu                      sync.RWMutex
}

type CSafeRule struct {
	Rules []Rule
	mu    sync.RWMutex
}

type Rule struct {
	Name         string
	Listen       uint16
	Forward      string
	Quota        int64
	ExpireDate   int64
	Simultaneous int
}
type Config struct {
	SaveDuration int
	Timeout      int64
	Rules        []Rule
}

//Timeout values
var EnableTimeOut = true
var TimeoutDuration time.Duration

func main() {
	{ //Parse arguments
		flag.StringVar(&ConfigFileName, "config", "rules.json", "The config filename")
		flag.IntVar(&Verbose, "verbose", 1, "Verbose level: 0->None(Mostly Silent), 1->Quota reached, expiry date and typical errors, 2->Connection flood 3->Timeout drops 4->All logs and errors")
		help := flag.Bool("h", false, "Show help")
		flag.BoolVar(&SaveBeforeExit, "no-exit-save", false, "Set this argument to disable the save of rules before exiting")
		flag.Parse()

		if *help {
			fmt.Println("Created by Hirbod Behnam")
			fmt.Println("Source at https://github.com/HirbodBehnam/PortForwarder")
			fmt.Println("Version", Version)
			flag.PrintDefaults()
			os.Exit(0)
		}

		if Verbose != 0 {
			fmt.Println("Verbose mode on level", Verbose)
		}

		SaveBeforeExit = !SaveBeforeExit
	}

	//Read config file
	var conf Config
	{
		confF, err := ioutil.ReadFile(ConfigFileName)
		if err != nil {
			panic("Cannot read the config file. (io Error) " + err.Error())
		}

		err = json.Unmarshal(confF, &conf)
		if err != nil {
			panic("Cannot read the config file. (Parse Error) " + err.Error())
		}

		Rules.Rules = conf.Rules
		SimultaneousConnections.SimultaneousConnections = make([]int, len(Rules.Rules))
		if conf.Timeout == -1 {
			logVerbose(1, "Disabled timeout")
			EnableTimeOut = false
		} else {
			TimeoutDuration = time.Duration(conf.Timeout) * time.Second
			logVerbose(3, "Set timeout to", TimeoutDuration)
		}
	}

	//Start listeners
	for index, rule := range Rules.Rules {
		go func(i int, loopRule Rule) {
			if loopRule.Quota < 0 { //If the quota is already reached why listen for connections?
				log.Println("Skip enabling forward on port", loopRule.Listen, "because the quota is reached.")
				return
			}
			if loopRule.ExpireDate != 0 && loopRule.ExpireDate < time.Now().Unix() {
				log.Println("Skip enabling forward on port", loopRule.Listen, "because this rule is expired.")
				return
			}

			log.Println("Forwarding from", loopRule.Listen, "port to", loopRule.Forward)
			ln, err := net.Listen("tcp", ":"+strconv.Itoa(int(loopRule.Listen))) //Listen on port
			if err != nil {
				panic(err)
			}

			for {
				conn, err := ln.Accept() //The loop will be held here

				Rules.mu.RLock()              //Lock the mutex to just read the quota
				if Rules.Rules[i].Quota < 0 { //Check the quota
					Rules.mu.RUnlock()
					logVerbose(1, "Quota reached for port", loopRule.Listen, "pointing to", loopRule.Forward)
					if err == nil {
						_ = conn.Close()
					}
					saveConfig(conf)
					break
				}
				if Rules.Rules[i].ExpireDate != 0 && Rules.Rules[i].ExpireDate < time.Now().Unix() {
					Rules.mu.RUnlock()
					logVerbose(1, "Expire date reached for port", loopRule.Listen, "pointing to", loopRule.Forward)
					if err == nil {
						_ = conn.Close()
					}
					saveConfig(conf)
					break
				}
				Rules.mu.RUnlock()

				if err != nil {
					println("Error on accepting connection:", err.Error())
					continue
				}

				go handleRequest(conn, i, loopRule)
			}
		}(index, rule)
	}

	//Save config file
	go func() {
		sd := conf.SaveDuration
		if sd == 0 {
			sd = 600
			conf.SaveDuration = 600
		}
		for {
			time.Sleep(time.Duration(sd) * time.Second) //Save file every x seconds
			saveConfig(conf)
		}
	}()

	//https://gobyexample.com/signals
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() { //This will wait for a signal
		<-sigs
		done <- true
	}()
	log.Println("Ctrl + C to stop")
	<-done
	if SaveBeforeExit {
		saveConfig(conf) //Save the config file one last time before exiting
	}
	log.Println("Exiting")
}

func saveConfig(config Config) {
	Rules.mu.RLock() //Lock to read the rules
	config.Rules = Rules.Rules
	b, _ := json.Marshal(config)
	Rules.mu.RUnlock()

	err := ioutil.WriteFile(ConfigFileName, b, 0644)
	if err != nil {
		logVerbose(1, "Error re-writing rules: ", err)
	} else {
		logVerbose(4, "Saved the config")
	}
}

func handleRequest(conn net.Conn, index int, r Rule) {
	//Send a clone of rules to here to avoid need of locking mutex
	SimultaneousConnections.mu.RLock()
	if r.Simultaneous != 0 && SimultaneousConnections.SimultaneousConnections[index] >= (r.Simultaneous*2) { //If we have reached quota just terminate the connection; 0 means no limits
		logVerbose(2, "Blocking new connection for port", r.Listen, "because the connection limit is reached. The current active connections count is", SimultaneousConnections.SimultaneousConnections[index]/2)
		SimultaneousConnections.mu.RUnlock()
		_ = conn.Close()
		return
	}
	SimultaneousConnections.mu.RUnlock()

	proxy, err := net.Dial("tcp", r.Forward) //Open a connection to remote host
	if err != nil {
		logVerbose(1, "Error on dialing remote host:", err.Error())
		_ = conn.Close()
		return
	}

	SimultaneousConnections.mu.Lock()
	SimultaneousConnections.SimultaneousConnections[index] += 2 //Two is added; One for client to server and another for server to client
	logVerbose(4, "Accepting a connection from", conn.RemoteAddr(), "; Now", SimultaneousConnections.SimultaneousConnections[index], "SimultaneousConnections")
	SimultaneousConnections.mu.Unlock()

	go copyIO(conn, proxy, index)
	go copyIO(proxy, conn, index)
}

func copyIO(src, dest net.Conn, index int) {
	defer src.Close()
	defer dest.Close()

	var r int64 //r is the amount of bytes transferred
	var err error

	if EnableTimeOut {
		r, err = copyBuffer(src, dest)
	} else {
		r, err = io.Copy(src, dest)
	}

	if err != nil {
		if strings.Contains(err.Error(), "i/o timeout") {
			logVerbose(3, "A connection timed out from", src.RemoteAddr(), "to", dest.RemoteAddr())
		} else {
			logVerbose(4, "Error on copyBuffer(Usually happens):", err.Error())
		}

	}

	Rules.mu.Lock() //Lock to change the amount of data transferred
	Rules.Rules[index].Quota -= r
	Rules.mu.Unlock()

	SimultaneousConnections.mu.Lock()
	SimultaneousConnections.SimultaneousConnections[index]-- //This will actually run twice
	logVerbose(4, "Closing a connection from", src.RemoteAddr(), "; Connections Now:", SimultaneousConnections.SimultaneousConnections[index])
	SimultaneousConnections.mu.Unlock()
}

func copyBuffer(dst, src net.Conn) (written int64, err error) {
	buf := make([]byte, 32768)
	for {
		err = src.SetDeadline(time.Now().Add(TimeoutDuration))
		if err != nil {
			logVerbose(1, "cannot set timeout for src")
			break
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			err = dst.SetDeadline(time.Now().Add(TimeoutDuration))
			if err != nil {
				logVerbose(1, "cannot set timeout for dest")
				break
			}
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

func logVerbose(level int, msg ...interface{}) {
	if Verbose >= level {
		log.Println(msg)
	}
}
