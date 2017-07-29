package main

import (
	"flag"
	"net"

	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

type stringMapVar struct {
	contents map[string]int
}

func (s stringMapVar) String() string {
	return fmt.Sprint(s.contents)
}

func (s stringMapVar) Set(value string) error {
	iface := strings.Split(value, ",")
	bandwidth, err := strconv.Atoi(iface[1])
	if err != nil {
		log.WithError(err).Fatal("Could not parse iface arguments! Uh-oh!")
		panic(err)
	}
	s.contents[iface[0]] = bandwidth
	return nil
}

var flagListenMode bool
var flagAddress string
var flagIfacePoll int
var flagRedundant bool
var flagRedundantDownload bool
var flagRedundantUpload bool
var (
	flagInterfaces = stringMapVar{
		contents: make(map[string]int),
	}
)

func init() {
	log.SetFormatter(&log.TextFormatter{})
	log.SetLevel(log.DebugLevel)

	flag.BoolVar(&flagRedundant, "r", false, "Send packets on every interface instead of just one? (Improves reliability)")
	flag.BoolVar(&flagRedundantDownload, "rd", false, "Redundant mode only for downloads")
	flag.BoolVar(&flagRedundantUpload, "ru", false, "Redundant mode only for uploads")
	flag.BoolVar(&flagListenMode, "l", false, "Should listen?")
	flag.StringVar(&flagAddress, "address", "localhost:5022", "Address of the server")
	flag.IntVar(&flagIfacePoll, "poll", 5, "How fast we should poll for new interfaces")
	flag.Var(flagInterfaces, "iface", "Specifies which interfaces will be used for connections, as well as the bandwidth for each interface.\n"+
		"Bandwidth is in KB/s. For unrestricted bandwidth, specify 0.\n"+
		"Usage: fusion -iface=<interface name>,<bandwidth>"+
		"Example: fusion -iface=en0,50")
	//flag.Var(flagUDPInterfaces, "udp-interface", "The name of the interface you wish to use as UDP instead of TCP (This is not recommended for a single interface, as retrying on UDP will not be attempted")
	//flag.Var(flagUDPInterfaces, "ui", "Shorthand for the 'udp-interface' parameter. See 'udp-interface' for usage")
	//flag.Memes
}

func main() {
	flag.Parse()
	log.Debug(flagInterfaces)
	if flagListenMode {
		if flagRedundant || flagRedundantUpload || flagRedundantDownload {
			log.WithFields(log.Fields{
				"redundant":      flagRedundant,
				"redundant-up":   flagRedundantUpload,
				"redundant-down": flagRedundantDownload,
			}).Warning("While in listen mode, the redundant flags will have no effect.")
			return
		}
		err := Server()
		if err != nil {
			panic(err)
		}
	} else {
		err := Client(flagAddress)
		if err != nil {
			panic(err)
		}
	}
}

func Client(serverAddr string) error {
	log.WithFields(log.Fields{
		"rud":    flagRedundant,
		"rud-up": flagRedundantUpload,
		"rud-dl": flagRedundantDownload,
		"listen": flagListenMode,
		"addr":   flagAddress,
		"poll":   flagIfacePoll,
	}).Info("Starting client...")
	ln, err := net.Listen("tcp", ":5021")
	if err != nil {
		return err
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go func() {
			sessionID := ClientReceivedSSHConnection(conn)
			err := SetupInterfaces(sessionID, serverAddr)
			if err != nil {
				log.Errorln(err)
			}
		}()
	}
}

func Server() error {
	log.WithFields(log.Fields{
		"listen": flagListenMode,
		"addr":   flagAddress,
		"poll":   flagIfacePoll,
	}).Info("Starting server...")
	ln, err := net.Listen("tcp", ":5022")
	if err != nil {
		return err
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go func() {
			err := ServerReceivedClientConnection(conn)
			if err != nil {
				// TODO: Add in and then utilize per-connection logging contexts
				log.WithFields(log.Fields{
					"conn": conn,
				}).Errorln(err)
			}
		}()
	}
}
