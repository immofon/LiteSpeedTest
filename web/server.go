package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xxf098/lite-proxy/web/render"
)

var upgrader = websocket.Upgrader{}

func ServeFile(port int) error {
	http.Handle("/", http.FileServer(http.FS(guiStatic)))
	http.HandleFunc("/test", updateTest)
	http.HandleFunc("/generateResult", generateResult)
	log.Printf("Start server at http://127.0.0.1:%d", port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	return err
}

// func ServeWasm(port int) error {
// 	http.Handle("/", http.FileServer(http.FS(wasmStatic)))
// 	log.Printf("Start server at http://127.0.0.1:%d", port)
// 	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
// 	return err
// }

func updateTest(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		// log.Printf("recv: %s", message)
		links, options, err := parseMessage(message)
		if err != nil {
			log.Println("parseMessage:", err)
			continue
		}
		p := ProfileTest{
			Writer:      c,
			MessageType: mt,
			Links:       links,
			Options:     options,
		}
		go p.testAll(ctx)
		// err = c.WriteMessage(mt, getMsgByte(0, "gotspeed"))
		// if err != nil {
		// 	log.Println("write:", err)
		// 	break
		// }
	}
}

func readConfig(configPath string) (*ProfileTestOptions, error) {
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	options := &ProfileTestOptions{}
	if err = json.Unmarshal(data, options); err != nil {
		return nil, err
	}
	if options.Concurrency < 1 {
		options.Concurrency = 1
	}
	if options.Language == "" {
		options.Language = "en"
	}
	if options.Theme == "" {
		options.Theme = "rainbow"
	}
	if options.Timeout < 8 {
		options.Timeout = 8
	}
	options.Timeout = options.Timeout * time.Second
	return options, nil
}

func TestFromCMD(subscription string, configPath *string, results chan Result) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	options := ProfileTestOptions{
		GroupName:     "Default",
		SpeedTestMode: "all",
		PingMethod:    "googleping",
		SortMethod:    "rspeed",
		Concurrency:   4,
		TestMode:      2,
		Subscription:  subscription,
		Language:      "en",
		FontSize:      24,
		Theme:         "rainbow",
		Timeout:       15 * time.Second,
		GeneratePic:   true,
	}
	if configPath != nil {
		if opt, err := readConfig(*configPath); err == nil {
			options = *opt
			options.GeneratePic = true
		}
	}
	if jsonOpt, err := json.Marshal(options); err == nil {
		log.Printf("json options: %s\n", string(jsonOpt))
	}
	links, err := parseLinks(subscription)
	if err != nil {
		return err
	}
	rand.Shuffle(len(links), func(i, j int) {
		links[i], links[j] = links[j], links[i]
	})

	outputMessageWriter := OutputMessageWriter{}
	p := ProfileTest{
		Writer:      &outputMessageWriter,
		Result:      results,
		MessageType: 1,
		Links:       links,
		Options:     &options,
	}
	return p.testAll(ctx)
}

type TestResult struct {
	TotalTraffic string `json:"totalTraffic"`
	TotalTime    string `json:"totalTime"`
	Language     string `json:"language"`
	FontSize     int    `json:"fontSize"`
	Theme        string `json:"theme"`
	// SortMethod   string       `json:"sortMethod"`
	Nodes render.Nodes `json:"nodes"`
}

func generateResult(w http.ResponseWriter, r *http.Request) {
	result := TestResult{}
	if r.Body == nil {
		http.Error(w, "Please send a request body", 400)
		return
	}
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Please send a request body", 400)
		return
	}
	if err = json.Unmarshal(data, &result); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	fontPath := "WenQuanYiMicroHei-01.ttf"
	options := render.NewTableOptions(40, 30, 0.5, 0.5, result.FontSize, 0.5, fontPath, result.Language, result.Theme, "Asia/Shanghai", FontBytes)
	table, err := render.NewTableWithOption(result.Nodes, &options)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	linksCount := 0
	successCount := 0
	for _, v := range result.Nodes {
		linksCount += 1
		if v.IsOk {
			successCount += 1
		}
	}
	msg := table.FormatTraffic(result.TotalTraffic, result.TotalTime, fmt.Sprintf("%d/%d", successCount, linksCount))
	if picdata, err := table.EncodeB64(msg); err == nil {
		fmt.Fprint(w, picdata)
	}

}
