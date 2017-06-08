package request_generators

import (
	"encoding/csv"
	"fmt"
	"github.com/v3io/http_blaster/httpblaster/config"
	"github.com/v3io/http_blaster/httpblaster/igz_data"
	"github.com/valyala/fasthttp"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
)

type Csv2KV struct {
	workload config.Workload
	base_uri string
	RequestCommon
}

func (self *Csv2KV) UseCommon(c RequestCommon) {

}

func (self *Csv2KV) generate_request(ch_records chan []string, ch_req chan *fasthttp.Request, host string,
	wg *sync.WaitGroup) {
	defer wg.Done()
	parser := igz_data.EmdSchemaParser{}
	var contentType string = "text/html"
	e := parser.LoadSchema(self.workload.Schema, self.workload.KeyFields, self.workload.KeyFormat)
	if e != nil {
		panic(e)
	}
	for r := range ch_records {
		json_payload := parser.EmdFromCSVRecord(r)
		req := self.PrepareRequest(contentType, self.workload.Header, string(self.workload.Type),
			self.base_uri, json_payload, host)
		ch_req <- req
	}
}

func (self *Csv2KV) generate(ch_req chan *fasthttp.Request, payload string, host string) {
	defer close(ch_req)
	var ch_records chan []string = make(chan []string)

	wg := sync.WaitGroup{}
	wg.Add(runtime.NumCPU())
	for c := 0; c < runtime.NumCPU(); c++ {
		go self.generate_request(ch_records, ch_req, host, &wg)
	}

	ch_files := self.FilesScan(self.workload.Payload)

	for f := range ch_files {
		fp, err := os.Open(f)
		if err != nil {
			panic(err)
		}

		r := csv.NewReader(fp)
		r.Comma = self.workload.Separator.Rune
		var line_count int=0
		for {
			record, err := r.Read()
			if err != nil {
				if err == io.EOF {
					break
				}
				panic(err)
			}
			if strings.HasPrefix(record[0], "#") {
				log.Println("Skipping scv header ", strings.Join(record[:], ","))
			} else {
				ch_records <- record
				line_count++
				if line_count % 500 == 0 {
					log.Printf("line: %d from file %s was submitted", line_count, f)
				}
			}
		}
		fp.Close()
	}

	close(ch_records)
	wg.Wait()
}

func (self *Csv2KV) GenerateRequests(wl config.Workload, tls_mode bool, host string) chan *fasthttp.Request {
	self.workload = wl
	//panic(fmt.Sprintf("workload key [%s] workload key sep [%s]", wl.KeyFormat, string(wl.KeyFormatSep.Rune)))
	if self.workload.Header == nil {
		self.workload.Header = make(map[string]string)
	}
	self.workload.Header["X-v3io-function"] = "PutItem"

	if tls_mode {
		self.base_uri = fmt.Sprintf("https://%s/%s/%s", host, self.workload.Bucket, self.workload.Target)
	} else {
		self.base_uri = fmt.Sprintf("http://%s/%s/%s", host, self.workload.Bucket, self.workload.Target)
	}
	ch_req := make(chan *fasthttp.Request, 1000)

	go self.generate(ch_req, self.workload.Payload, host)

	return ch_req
}
