package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"sync"
)

type Pool struct {
	Address    string `json:"address"`
	Proportion int    `json:"proportion"`
}

type Config struct {
	Pools []Pool `json:"pools"`
}

var config Config
var poolConnections map[string]net.Conn
var mutex sync.Mutex

func loadConfig() error {
	data, err := ioutil.ReadFile("config.json")
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &config)
}

func selectPool() string {
	total := 0
	for _, pool := range config.Pools {
		total += pool.Proportion
	}
	randValue := rand.Intn(total)
	for _, pool := range config.Pools {
		if randValue < pool.Proportion {
			return pool.Address
		}
		randValue -= pool.Proportion
	}
	return config.Pools[0].Address
}

func handleClient(clientConn net.Conn) {
	poolAddress := selectPool()
	log.Printf("Forwarding connection to: %s", poolAddress)

	mutex.Lock()
	poolConn, exists := poolConnections[poolAddress]
	if !exists {
		var err error
		poolConn, err = net.Dial("tcp", poolAddress)
		if err != nil {
			log.Printf("Failed to connect to pool %s: %v", poolAddress, err)
			clientConn.Close()
			mutex.Unlock()
			return
		}
		poolConnections[poolAddress] = poolConn
	}
	mutex.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer clientConn.Close()
		ioCopyWithLogging(clientConn, poolConn)
	}()

	go func() {
		defer wg.Done()
		defer clientConn.Close()
		ioCopyWithLogging(poolConn, clientConn)
	}()

	wg.Wait()

	mutex.Lock()
	if poolConn != nil {
		poolConn.Close()
		delete(poolConnections, poolAddress)
	}
	mutex.Unlock()
}

func ioCopyWithLogging(dst net.Conn, src net.Conn) {
	_, err := io.Copy(dst, src)
	if err != nil {
		if err == io.EOF {
			log.Printf("Connection closed by %v", src.RemoteAddr())
		} else {
			log.Printf("Error copying data from %v to %v: %v", src.RemoteAddr(), dst.RemoteAddr(), err)
		}
	}
}

func main() {
	if err := loadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	poolConnections = make(map[string]net.Conn)
	listener, err := net.Listen("tcp", ":3333")
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer listener.Close()

	log.Println("Stratum proxy listening on :3333")

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}
		go handleClient(clientConn)
	}
}
