package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math"
	"sync"

	"github.com/jfreymuth/pulse" // for pulseaudio

	"google.golang.org/protobuf/proto"

	storage "github.com/synerex/proto_storage"
	api "github.com/synerex/synerex_api"
	pbase "github.com/synerex/synerex_proto"
	sxutil "github.com/synerex/synerex_sxutil"
)

var (
	nodesrv      = flag.String("nodesrv", "127.0.0.1:9990", "Node ID Server")
	local        = flag.String("local", "", "Specify Local Synerex Server")
	fragment     = flag.Int("fragment", 2048, "Audio capture fragment size")
	rate         = flag.Int("rate", 44100, "Audio capture sampling rate")
	verbose      = flag.Bool("verbose", false, "Verbose audio information")
	lastUnixNano int64
	sxclient     *sxutil.SXServiceClient
	clt          *pulse.Client
)

func init() {
	opts := []pulse.ClientOption{
		pulse.ClientApplicationName("SxProviderRX"),
	}
	var err error
	clt, err = pulse.NewClient(opts...)
	if err != nil {
		fmt.Println("Pulseaudio init erorr", err)
	} else {
		log.Printf("Pulseaudio init %##v", clt)
	}
}

func bytes2float32(bt []byte) []float32 {
	floats := make([]float32, len(bt)/4)
	for i := 0; i < len(floats); i++ {
		ui := binary.LittleEndian.Uint32(bt[i*4:])
		floats[i] = math.Float32frombits(ui)
	}
	return floats
}

func playAudio(p []float32) {

	reader := func(out []float32) (int, error) {
		log.Printf("reader! len %d %d", len(out), len(p))
		for i := range out {
			if i < len(p) {
				out[i] = p[i]
			} else {
				return i, nil
			}
		}
		return len(out), nil
	}

	//	log.Printf("Working %#v", p)
	//	log.Printf("clt %#v", clt)
	stream, err := clt.NewPlayback(pulse.Float32Reader(reader), pulse.PlaybackLatency(.1))
	if err != nil {
		fmt.Println("err playback:", err)
	}
	stream.Start()

	// no grace close..( c.close, stream.stop)
}

func supplyAudioCallback(clt *sxutil.SXServiceClient, sp *api.Supply) {

	record := &storage.Record{}
	if sp.Cdata != nil {
		err := proto.Unmarshal(sp.Cdata.Entity, record)
		if err == nil { // get Audio Data
			playAudio(bytes2float32(record.Record))
		}
	}
	//	log.Printf("Unmarshal error on View_Pcoutner %s", sp.SupplyName)
}

func SubscribeAudio(client *sxutil.SXServiceClient) {
	sxutil.SimpleSubscribeSupply(client, supplyAudioCallback)
}

func main() {
	log.Printf("PulseAudio Receiver Provider(%s) built %s sha1 %s", sxutil.GitVer, sxutil.BuildTime, sxutil.Sha1Ver)
	flag.Parse()
	go sxutil.HandleSigInt()
	sxutil.RegisterDeferFunction(sxutil.UnRegisterNode)

	//	channelTypes := []uint32{pbase.MEDIA_SVC}
	channelTypes := []uint32{pbase.STORAGE_SERVICE}
	srv, rerr := sxutil.RegisterNode(*nodesrv, "PulseAudioRx", channelTypes, nil)

	if rerr != nil {
		log.Fatal("Can't register node:", rerr)
	}
	if *local != "" { // quick hack for AWS local network
		srv = *local
	}
	log.Printf("Connecting SynerexServer at [%s]", srv)

	//	wg := sync.WaitGroup{} // for syncing other goroutines

	gclient := sxutil.GrpcConnectServer(srv)

	if gclient == nil {
		log.Fatal("Can't connect Synerex Server")
	} else {
		log.Print("Connecting SynerexServer")
	}

	argJson := fmt.Sprintf("{PulseAudioRx}", *rate, *fragment)

	//	sxclient = sxutil.NewSXServiceClient(gclient, pbase.MEDIA_SVC, argJson)
	sxclient = sxutil.NewSXServiceClient(gclient, pbase.STORAGE_SERVICE, argJson)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go SubscribeAudio(sxclient)

	wg.Wait()
}
