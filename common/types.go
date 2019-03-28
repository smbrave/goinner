package common

import "strings"

type StringArray []string

func (a *StringArray) Get() interface{} { return []string(*a) }

func (a *StringArray) Set(s string) error {
	*a = append(*a, s)
	return nil
}

func (a *StringArray) String() string {
	return strings.Join(*a, ",")
}

var (
	PACKET_CODE_KEEPALIVE = 1
	PACKET_CODE_CONNECT   = 2
)

type Keepalive struct {
	Code int    `json:"code"`
	Addr string `json:"addr"`
}
