package loadlab

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/transport"
	"github.com/li41/astrahold-server/internal/world"
)

type BotConfig struct {
	TCPAddress     string
	Clients        int
	Scenario       Scenario
	InputRateHz    int
	RampUp         time.Duration
	ConnectTimeout time.Duration
}

type BotReport struct {
	SchemaVersion            int             `json:"schema_version"`
	Scenario                 Scenario        `json:"scenario"`
	RequestedClients         int             `json:"requested_clients"`
	ConnectedClients         uint64          `json:"connected_clients"`
	ReadyClients             uint64          `json:"ready_clients"`
	FailedConnections        uint64          `json:"failed_connections"`
	DurationSeconds          float64         `json:"duration_seconds"`
	ConnectionLatency        DurationSummary `json:"connection_latency"`
	MovesSent                uint64          `json:"moves_sent"`
	UDPBytesSent             uint64          `json:"udp_bytes_sent"`
	UDPBytesReceived         uint64          `json:"udp_bytes_received"`
	TCPBytesReceived         uint64          `json:"tcp_bytes_received"`
	ReliableMessages         uint64          `json:"reliable_messages"`
	RealtimeMessages         uint64          `json:"realtime_messages"`
	Snapshots                uint64          `json:"snapshots"`
	CompletedSnapshots       uint64          `json:"completed_snapshots"`
	IncompleteSnapshotResets uint64          `json:"incomplete_snapshot_resets"`
	Corrections              uint64          `json:"corrections"`
	Spawns                   uint64          `json:"spawns"`
	Despawns                 uint64          `json:"despawns"`
	DynamicStates            uint64          `json:"dynamic_states"`
	DecodeErrors             uint64          `json:"decode_errors"`
	NetworkErrors            uint64          `json:"network_errors"`
}

type botCollector struct {
	connected         atomic.Uint64
	ready             atomic.Uint64
	failedConnections atomic.Uint64
	moves             atomic.Uint64
	udpSent           atomic.Uint64
	udpReceived       atomic.Uint64
	tcpReceived       atomic.Uint64
	reliable          atomic.Uint64
	realtime          atomic.Uint64
	snapshots         atomic.Uint64
	completedSnapshots atomic.Uint64
	incompleteSnapshotResets atomic.Uint64
	corrections       atomic.Uint64
	spawns            atomic.Uint64
	despawns          atomic.Uint64
	dynamicStates     atomic.Uint64
	decodeErrors      atomic.Uint64
	networkErrors     atomic.Uint64

	latencyMu sync.Mutex
	latencies []time.Duration
}

func RunBots(ctx context.Context, config BotConfig) (BotReport, error) {
	if config.TCPAddress == "" || config.Clients <= 0 || config.InputRateHz <= 0 {
		return BotReport{}, errors.New("loadlab: invalid bot config")
	}
	if _, err := ParseScenario(string(config.Scenario)); err != nil {
		return BotReport{}, err
	}
	if config.ConnectTimeout <= 0 {
		config.ConnectTimeout = 5 * time.Second
	}

	started := time.Now()
	collector := &botCollector{latencies: make([]time.Duration, 0, config.Clients)}
	var wg sync.WaitGroup
	for i := 0; i < config.Clients; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			if config.RampUp > 0 && config.Clients > 1 {
				delay := time.Duration(int64(config.RampUp) * int64(index) / int64(config.Clients-1))
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			if err := runBot(ctx, config, collector); err != nil {
				collector.networkErrors.Add(1)
			}
		}(i)
	}
	wg.Wait()

	collector.latencyMu.Lock()
	latencies := append([]time.Duration(nil), collector.latencies...)
	collector.latencyMu.Unlock()

	return BotReport{
		SchemaVersion:            ReportSchemaVersion,
		Scenario:                 config.Scenario,
		RequestedClients:         config.Clients,
		ConnectedClients:         collector.connected.Load(),
		ReadyClients:             collector.ready.Load(),
		FailedConnections:        collector.failedConnections.Load(),
		DurationSeconds:          time.Since(started).Seconds(),
		ConnectionLatency:        summarizeDurations(latencies),
		MovesSent:                collector.moves.Load(),
		UDPBytesSent:             collector.udpSent.Load(),
		UDPBytesReceived:         collector.udpReceived.Load(),
		TCPBytesReceived:         collector.tcpReceived.Load(),
		ReliableMessages:         collector.reliable.Load(),
		RealtimeMessages:         collector.realtime.Load(),
		Snapshots:                collector.snapshots.Load(),
		CompletedSnapshots:       collector.completedSnapshots.Load(),
		IncompleteSnapshotResets: collector.incompleteSnapshotResets.Load(),
		Corrections:              collector.corrections.Load(),
		Spawns:                   collector.spawns.Load(),
		Despawns:                 collector.despawns.Load(),
		DynamicStates:            collector.dynamicStates.Load(),
		DecodeErrors:             collector.decodeErrors.Load(),
		NetworkErrors:            collector.networkErrors.Load(),
	}, nil
}

func runBot(ctx context.Context, config BotConfig, collector *botCollector) error {
	dialStarted := time.Now()
	dialer := net.Dialer{Timeout: config.ConnectTimeout}
	raw, err := dialer.DialContext(ctx, "tcp", config.TCPAddress)
	if err != nil {
		collector.failedConnections.Add(1)
		return err
	}
	counted := &countingConn{Conn: raw}
	defer func() {
		_ = counted.Close()
		collector.tcpReceived.Add(counted.readBytes.Load())
	}()
	if tcp, ok := raw.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	collector.connected.Add(1)
	collector.latencyMu.Lock()
	collector.latencies = append(collector.latencies, time.Since(dialStarted))
	collector.latencyMu.Unlock()

	codec := gamev1.Codec{}
	welcomeEnvelope, err := transport.ReadEnvelope(counted, codec)
	if err != nil {
		return err
	}
	welcome, ok := welcomeEnvelope.Message.(protocol.SessionWelcome)
	if !ok || welcomeEnvelope.Delivery != protocol.DeliveryReliableOrdered || !welcome.World.Valid() {
		return errors.New("loadlab: invalid SessionWelcome")
	}
	token, err := tcpudp.ParseToken(welcome.RealtimeToken)
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(raw.RemoteAddr().String())
	if err != nil {
		return err
	}
	udpAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(int(welcome.RealtimePort))))
	if err != nil {
		return err
	}
	udp, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return err
	}
	defer udp.Close()
	collector.ready.Add(1)

	botCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		reliableReadLoop(botCtx, cancel, counted, codec, collector)
	}()
	go func() {
		defer readers.Done()
		udpReadLoop(botCtx, udp, token, codec, collector)
	}()
	defer func() {
		cancel()
		_ = counted.Close()
		_ = udp.Close()
		readers.Wait()
	}()

	interval := time.Second / time.Duration(config.InputRateHz)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	started := time.Now()
	var sequence uint32

	// 第一包立刻送出，讓 Server 綁定 realtime endpoint，不等待第一個 ticker。
	sequence++
	if err := sendMove(udp, token, codec, config.Scenario, welcome.EntityID, sequence, time.Since(started), collector); err != nil {
		return err
	}

	for {
		select {
		case <-botCtx.Done():
			return nil
		case <-ticker.C:
			sequence++
			if err := sendMove(udp, token, codec, config.Scenario, welcome.EntityID, sequence, time.Since(started), collector); err != nil {
				if ctx.Err() == nil {
					collector.networkErrors.Add(1)
				}
				return nil
			}
		}
	}
}

func sendMove(udp *net.UDPConn, token tcpudp.Token, codec transport.PayloadCodec, scenario Scenario, entityID world.EntityID, sequence uint32, elapsed time.Duration, collector *botCollector) error {
	dx, dz := MovementDirection(scenario, entityID, elapsed)
	envelope := protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: sequence,
		Message:  protocol.ClientMoveInput{DirectionX: dx, DirectionZ: dz},
	}
	packet, err := tcpudp.EncodeDatagram(token, envelope, codec)
	if err != nil {
		return err
	}
	n, err := udp.Write(packet)
	if err != nil {
		return err
	}
	collector.moves.Add(1)
	collector.udpSent.Add(uint64(n))
	return nil
}

func reliableReadLoop(ctx context.Context, cancel context.CancelFunc, conn net.Conn, codec transport.PayloadCodec, collector *botCollector) {
	for {
		envelope, err := transport.ReadEnvelope(conn, codec)
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				collector.networkErrors.Add(1)
			}
			cancel()
			return
		}
		collector.reliable.Add(1)
		countNonSnapshotMessage(envelope.Message, collector)
	}
}

func udpReadLoop(ctx context.Context, udp *net.UDPConn, expectedToken tcpudp.Token, codec transport.PayloadCodec, collector *botCollector) {
	buffer := make([]byte, tcpudp.MaxDatagramSize)
	assembler := snapshotAssembly{}
	for {
		n, err := udp.Read(buffer)
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
				collector.networkErrors.Add(1)
			}
			return
		}
		collector.udpReceived.Add(uint64(n))
		token, envelope, err := tcpudp.DecodeDatagram(buffer[:n], codec)
		if err != nil || token != expectedToken {
			collector.decodeErrors.Add(1)
			continue
		}
		collector.realtime.Add(1)
		if snapshot, ok := envelope.Message.(protocol.WorldSnapshot); ok {
			collector.snapshots.Add(1)
			complete, reset := assembler.Accept(snapshot)
			if reset {
				collector.incompleteSnapshotResets.Add(1)
			}
			if complete {
				collector.completedSnapshots.Add(1)
			}
			continue
		}
		countNonSnapshotMessage(envelope.Message, collector)
	}
}

func countNonSnapshotMessage(message protocol.Message, collector *botCollector) {
	switch message.(type) {
	case protocol.PositionCorrection:
		collector.corrections.Add(1)
	case protocol.EntitySpawn:
		collector.spawns.Add(1)
	case protocol.EntityDespawn:
		collector.despawns.Add(1)
	case protocol.WorldDynamicState:
		collector.dynamicStates.Add(1)
	}
}

type snapshotAssembly struct {
	tick             uint64
	chunkCount       uint16
	received         []bool
	receivedCount    int
	lastCompleteTick uint64
}

func (a *snapshotAssembly) Accept(snapshot protocol.WorldSnapshot) (complete bool, resetIncomplete bool) {
	if !snapshot.ValidChunk() || snapshot.Tick <= a.lastCompleteTick {
		return false, false
	}
	if a.tick != snapshot.Tick {
		resetIncomplete = a.tick != 0 && a.receivedCount < int(a.chunkCount)
		a.tick = snapshot.Tick
		a.chunkCount = snapshot.ChunkCount
		a.received = make([]bool, int(snapshot.ChunkCount))
		a.receivedCount = 0
	}
	if snapshot.ChunkCount != a.chunkCount || int(snapshot.ChunkIndex) >= len(a.received) {
		return false, resetIncomplete
	}
	if !a.received[snapshot.ChunkIndex] {
		a.received[snapshot.ChunkIndex] = true
		a.receivedCount++
	}
	if a.receivedCount != int(a.chunkCount) {
		return false, resetIncomplete
	}
	a.lastCompleteTick = a.tick
	a.tick = 0
	a.chunkCount = 0
	a.received = nil
	a.receivedCount = 0
	return true, resetIncomplete
}

type countingConn struct {
	net.Conn
	readBytes atomic.Uint64
}

func (c *countingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.readBytes.Add(uint64(n))
	}
	return n, err
}
