package main

import (
	"flag"
	"log"
	"strings"

	"github.com/smbrave/goinner/common"
)

func main() {
	var proxys common.StringArray
	flag.Var(&proxys, "proxy", "address map for example --proxy=0.0.0.0:50000,0.0.0.0:40000,127.0.0.1:22")
	flag.Parse()

	log.Println(proxys)

	for _, m := range proxys {
		fields := strings.Split(m, ",")
		if len(fields) != 3 {
			log.Println("--proxy=", m, "field error")
			continue
		}

		s := NewServer(&Config{
			FrontAddr:   fields[0],
			BackendAddr: fields[1],
			RemoteAddr:  fields[2],
		})

		log.Println("start proxy:", fields[0], "==>", fields[1], "==>", fields[2])
		go s.Start()
	}

	select {}
}
