package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/c2h5oh/datasize"
	webServer "github.com/xxf098/lite-proxy/web"
)

var (
	u     = flag.String("url", "http://localhost:8090/", "server url to post information")
	port  = flag.Int("p", 8090, "set port")
	test  = flag.String("test", "", "test from command line with subscription link or file")
	conf  = flag.String("config", "", "command line options")
	token = flag.String("token", "", "token")
	s     = flag.String("s", "server", "status: server or client")
)

type Node struct {
	Name     string
	Link     string
	Protocol string
	Ping     int
	AvgSpeed int
	MaxSpeed int
	AddedAt  time.Time
}

func client() {
	results := make(chan webServer.Result, 1000)
	go func() {
		for r := range results {
			log.Printf("%s %d %d\n", r.Name, r.AvgSpeed, r.MaxSpeed)
			if strings.Contains(r.Name, "中国") {
				continue
			}

			data, err := json.Marshal(r)
			if err != nil {
				log.Print(err)
				continue
			}
			resp, err := http.Post(*u, "application/json", bytes.NewReader(data))
			if resp.StatusCode != http.StatusCreated {
				log.Printf("resp: %s\n", resp.Status)
			}
			if err != nil {
				log.Print(err)
				continue
			}
		}
	}()

	if *test == "" {
		log.Fatal("You MUST set -test")
	}

	for {
		func() {
			defer func() {
				r := recover()
				if r != nil {
					log.Print("panic: ", r)
				}
			}()

			if err := webServer.TestFromCMD(*test, conf, results); err != nil {
				log.Print(err)
			}
		}()
		time.Sleep(time.Minute * 10)
	}

}
func server() {
	var (
		mu         *sync.Mutex = new(sync.Mutex)
		nodes      []Node      = make([]Node, 0, 31)
		max_length             = 30
	)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Only support GET method", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Refresh", "1;url=/")
		w.WriteHeader(http.StatusOK)
		mu.Lock()
		defer mu.Unlock()

		for i, node := range nodes {
			fmt.Fprintf(w, "%d:%s ping:%d avg-speed:%s, max-speed:%s\n", i+1, node.Name, node.Ping,
				datasize.ByteSize(node.AvgSpeed).HumanReadable(),
				datasize.ByteSize(node.MaxSpeed).HumanReadable())
		}
	})

	http.HandleFunc("/nodes/"+(*token), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			mu.Lock()
			defer mu.Unlock()
			w.WriteHeader(http.StatusOK)
			buf := bytes.NewBuffer(nil)
			for _, node := range nodes {
				fmt.Fprintf(buf, "%s\n", node.Link)
			}
			fmt.Fprintln(w, base64.URLEncoding.EncodeToString(buf.Bytes()))
			return
		}
		if r.Method == "POST" {
			var node Node
			err := json.NewDecoder(r.Body).Decode(&node)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if node.MaxSpeed <= 0 || node.Ping >= 10000 || node.Link == "" || node.AvgSpeed <= 0 {
				http.Error(w, "input error", http.StatusBadRequest)
				return
			}
			node.AddedAt = time.Now()

			mu.Lock()
			defer mu.Unlock()

			added := false
			for i, n := range nodes {
				if n.Link == node.Link {
					nodes[i] = node
					added = true
				}
			}

			if !added {
				nodes = append(nodes, node)
			}

			now := time.Now()
			sort.SliceStable(nodes, func(i, j int) bool {
				if now.Sub(nodes[j].AddedAt) > 15*time.Minute {
					return true
				}
				return nodes[i].AvgSpeed > nodes[j].AvgSpeed
			})
			if len(nodes) > max_length {
				nodes = nodes[:max_length]
			}

			w.WriteHeader(http.StatusCreated)
		}
	})

	listen := fmt.Sprintf("0.0.0.0:%d", *port)
	log.Println("Listen", listen)

	err := http.ListenAndServe(listen, nil)
	if err != nil {
		log.Fatal(err)
	}
}
func main() {
	flag.Parse()

	if *s == "server" {
		server()
		return
	}

	if *s == "client" {
		client()
		return
	}

}
