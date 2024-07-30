package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Pool struct {
	Address    string `json:"address"`
	Proportion int    `json:"proportion"`
}

type Config struct {
	Pools []Pool `json:"pools"`
}

var (
	config    Config
	poolUsage = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "stratum_proxy_pool_usage_total",
			Help: "Total number of connections to each pool",
		},
		[]string{"pool"},
	)
	logMessages bool
)

func init() {
	prometheus.MustRegister(poolUsage)
	logMessages = os.Getenv("LOG_MESSAGES") == "true"
}

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
	defer clientConn.Close()
	poolAddress := selectPool()
	log.Printf("Forwarding connection to: %s", poolAddress)

	poolUsage.WithLabelValues(poolAddress).Inc()

	poolConn, err := net.Dial("tcp", poolAddress)
	if err != nil {
		log.Printf("Failed to connect to pool %s: %v", poolAddress, err)
		return
	}
	defer poolConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := transferData(clientConn, poolConn, "client_to_pool"); err != nil {
			log.Printf("Error transferring data from client to pool: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := transferData(poolConn, clientConn, "pool_to_client"); err != nil {
			log.Printf("Error transferring data from pool to client: %v", err)
		}
	}()

	wg.Wait()
}

func transferData(src net.Conn, dst net.Conn, direction string) error {
	buffer := make([]byte, 1024)
	for {
		n, err := src.Read(buffer)
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
		if logMessages {
			logData := map[string]interface{}{
				"direction": direction,
				"bytes":     n,
				"message":   string(buffer[:n]),
			}
			logDataJSON, _ := json.Marshal(logData)
			log.Println(string(logDataJSON))
		}
		if _, err := dst.Write(buffer[:n]); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	rand.Seed(time.Now().UnixNano())

	if err := loadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(":2112", nil))
	}()

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
