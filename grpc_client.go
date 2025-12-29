package main

import (
	"context"
	"io"
	"log"
	"sync"
	"time"

	pb "elrs-map/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TelemetryState holds the latest telemetry data
type TelemetryState struct {
	sync.RWMutex

	// GPS
	Latitude    float32
	Longitude   float32
	Altitude    int32
	GroundSpeed float32
	Heading     float32
	Satellites  uint32
	HasGPS      bool

	// Attitude
	Pitch float32
	Roll  float32
	Yaw   float32

	// Battery
	Voltage   float32
	Current   float32
	Capacity  uint32
	Remaining uint32

	// Link stats
	RSSI1       int32
	RSSI2       int32
	LinkQuality uint32
	SNR         int32
	TXPower     uint32

	// Barometer
	BaroAltitude  float32
	VerticalSpeed float32

	// Flight mode
	FlightMode string

	// Connection state
	Connected   bool
	LinkStarted bool
	LastUpdate  time.Time
}

// GRPCClient manages connection to the elrs-joystick-control backend
type GRPCClient struct {
	addr   string
	conn   *grpc.ClientConn
	client pb.JoystickControlClient

	state     *TelemetryState
	ctx       context.Context
	cancel    context.CancelFunc
	streaming bool
	mu        sync.Mutex
}

// NewGRPCClient creates a new gRPC client
func NewGRPCClient(addr string) *GRPCClient {
	return &GRPCClient{
		addr:  addr,
		state: &TelemetryState{},
	}
}

// Connect establishes connection to the gRPC server
func (c *GRPCClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil // Already connected
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return err
	}

	c.conn = conn
	c.client = pb.NewJoystickControlClient(conn)
	c.state.Connected = true
	log.Printf("Connected to gRPC server at %s", c.addr)
	return nil
}

// Disconnect closes the gRPC connection
func (c *GRPCClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.StopTelemetryStream()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.client = nil
	}
	c.state.Connected = false
}

// GetTransmitters returns available serial ports
func (c *GRPCClient) GetTransmitters() ([]string, error) {
	if c.client == nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.client.GetTransmitters(ctx, &pb.Empty{})
	if err != nil {
		return nil, err
	}

	var ports []string
	for _, t := range resp.Transmitters {
		ports = append(ports, t.Port)
	}
	return ports, nil
}

// StartLink begins communication with the ELRS TX
func (c *GRPCClient) StartLink(port string, baudRate int32) error {
	if c.client == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.client.StartLink(ctx, &pb.StartLinkReq{
		Port:     port,
		BaudRate: baudRate,
	})
	if err != nil {
		return err
	}

	c.state.Lock()
	c.state.LinkStarted = true
	c.state.Unlock()

	log.Printf("Link started on %s @ %d baud", port, baudRate)
	return nil
}

// StopLink stops communication with the ELRS TX
func (c *GRPCClient) StopLink() error {
	if c.client == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.client.StopLink(ctx, &pb.Empty{})
	if err != nil {
		return err
	}

	c.state.Lock()
	c.state.LinkStarted = false
	c.state.Unlock()

	log.Println("Link stopped")
	return nil
}

// StartTelemetryStream begins streaming telemetry data
func (c *GRPCClient) StartTelemetryStream() error {
	c.mu.Lock()
	if c.streaming {
		c.mu.Unlock()
		return nil
	}
	c.streaming = true
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.mu.Unlock()

	go c.streamTelemetry()
	return nil
}

// StopTelemetryStream stops the telemetry stream
func (c *GRPCClient) StopTelemetryStream() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
	}
	c.streaming = false
}

func (c *GRPCClient) streamTelemetry() {
	for {
		c.mu.Lock()
		if !c.streaming || c.client == nil {
			c.mu.Unlock()
			return
		}
		ctx := c.ctx
		client := c.client
		c.mu.Unlock()

		stream, err := client.GetTelemetryStream(ctx, &pb.Empty{})
		if err != nil {
			log.Printf("Telemetry stream error: %v", err)
			time.Sleep(time.Second)
			continue
		}

		for {
			telem, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				if ctx.Err() != nil {
					return // Context cancelled, exit gracefully
				}
				log.Printf("Telemetry recv error: %v", err)
				break
			}

			c.processTelemetry(telem)
		}
	}
}

func (c *GRPCClient) processTelemetry(t *pb.Telemetry) {
	c.state.Lock()
	defer c.state.Unlock()

	c.state.LastUpdate = time.Now()

	switch data := t.Data.(type) {
	case *pb.Telemetry_Gps:
		c.state.Latitude = data.Gps.Latitude
		c.state.Longitude = data.Gps.Longitude
		c.state.Altitude = data.Gps.Altitude
		c.state.GroundSpeed = data.Gps.GroundSpeed
		c.state.Heading = data.Gps.Heading
		c.state.Satellites = data.Gps.Satellites
		c.state.HasGPS = true

	case *pb.Telemetry_Attitude:
		c.state.Pitch = data.Attitude.Pitch
		c.state.Roll = data.Attitude.Roll
		c.state.Yaw = data.Attitude.Yaw

	case *pb.Telemetry_Battery:
		c.state.Voltage = data.Battery.Voltage
		c.state.Current = data.Battery.Current
		c.state.Capacity = data.Battery.Capacity
		c.state.Remaining = data.Battery.Remaining

	case *pb.Telemetry_LinkStats:
		c.state.RSSI1 = data.LinkStats.Rssi1
		c.state.RSSI2 = data.LinkStats.Rssi2
		c.state.LinkQuality = data.LinkStats.LinkQuality
		c.state.SNR = data.LinkStats.Snr
		c.state.TXPower = data.LinkStats.TxPower

	case *pb.Telemetry_Barometer:
		c.state.BaroAltitude = data.Barometer.Altitude

	case *pb.Telemetry_Variometer:
		c.state.VerticalSpeed = data.Variometer.VerticalSpeed

	case *pb.Telemetry_BarometerVariometer:
		c.state.BaroAltitude = data.BarometerVariometer.Altitude
		c.state.VerticalSpeed = data.BarometerVariometer.VerticalSpeed

	case *pb.Telemetry_FlightMode:
		c.state.FlightMode = data.FlightMode.Mode
	}
}

// GetState returns a copy of the current telemetry state
func (c *GRPCClient) GetState() TelemetryState {
	c.state.RLock()
	defer c.state.RUnlock()
	return *c.state
}

// IsConnected returns true if connected to the gRPC server
func (c *GRPCClient) IsConnected() bool {
	c.state.RLock()
	defer c.state.RUnlock()
	return c.state.Connected
}

// IsLinkStarted returns true if the ELRS link is active
func (c *GRPCClient) IsLinkStarted() bool {
	c.state.RLock()
	defer c.state.RUnlock()
	return c.state.LinkStarted
}
