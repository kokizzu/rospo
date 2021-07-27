package pipeapi

import (
	"net"

	"github.com/ferama/rospo/pkg/utils"
)

type responseItem struct {
	ID       int            `json:"Id"`
	Listener net.Addr       `json:"Listener"`
	Endpoint utils.Endpoint `json:"Endpoint"`
}