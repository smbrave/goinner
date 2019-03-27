package main

import (
	"flag"
	"log"

	"strings"

	"strconv"

	"github.com/smbrave/goinner/common"
)

func main() {

	var proxys common.StringArray

	flag.Var(&proxys, "proxy", "address proxy for example --proxy=0.0.0.0:50000,0.0.0.0:40000,10")
	flag.Parse()

	log.Println(proxys)

	for _, m := range proxys {

		fields := strings.Split(m, ",")
		if len(fields) != 3 {
			log.Println("--proxy=", m, "field error")
			continue
		}
		concurrency, _ := strconv.ParseInt(fields[2], 10, 64)
		if concurrency == 0 {
			concurrency = 10
		}
		p := NewProxy(&Config{
			UpAddr:      fields[0],
			DownAddr:    fields[1],
			Concurrency: concurrency,
		})
		go p.Start()
	}

	select {}

}
