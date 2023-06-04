package main

import (
	"flag"
	"log"
	"net"

	"github.com/datachassis/go-nbd/pkg/backend"
	"github.com/datachassis/go-nbd/pkg/server"
)

func main() {
	size := flag.Int64("size", 1073741824, "Size of the memory region to expose")
	laddr := flag.String("laddr", ":10809", "Listen address")
	network := flag.String("network", "tcp", "Listen network (e.g. `tcp` or `unix`)")
	name := flag.String("name", "default", "Export name")
	description := flag.String("description", "The default export", "Export description")
	readOnly := flag.Bool("read-only", false, "Whether the export should be read-only")
	minimumBlockSize := flag.Uint("minimum-block-size", 1, "Minimum block size")
	preferredBlockSize := flag.Uint("preferred-block-size", 4096, "Preferred block size")
	maximumBlockSize := flag.Uint("maximum-block-size", 0xffffffff, "Maximum block size")

	flag.Parse()

	l, err := net.Listen(*network, *laddr)
	if err != nil {
		panic(err)
	}
	defer l.Close()

	log.Printf("Listening on [%s]", l.Addr())

	b := backend.NewMemoryBackend(make([]byte, *size))

	clients := 0
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Println("Could not accept connection, continuing:", err)

			continue
		}

		clients++

		log.Printf("%v clients connected", clients)

		go func() {
			defer func() {
				_ = conn.Close()

				clients--

				if err := recover(); err != nil {
					log.Printf("Client disconnected with error: %v", err)
				}

				log.Printf("%v clients connected", clients)
			}()

			if err := server.Handle(
				conn,
				[]server.Export{
					{
						Name:        *name,
						Description: *description,
						Backend:     b,
					},
				},
				&server.Options{
					ReadOnly:           *readOnly,
					MinimumBlockSize:   uint32(*minimumBlockSize),
					PreferredBlockSize: uint32(*preferredBlockSize),
					MaximumBlockSize:   uint32(*maximumBlockSize),
				}); err != nil {
				panic(err)
			}
		}()
	}
}
