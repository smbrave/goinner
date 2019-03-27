package main

import "flag"

var (
	flagFront      = flag.String("front", "", "front addrees")
	flagBackup    = flag.String("backup", "", "backup address")

)

func main() {
	flag.Parse()
	s := NewServer(&Config{
		FrontAddr:  *flagFront,
		BackendAddr: *flagBackup,
	})
	s.Start()
}
