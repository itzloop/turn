// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT
package stats

import (
	"net"
	"time"

	"github.com/pion/stun/v3"
)

type StatsMessageClass stun.MessageClass

const (
	ClassUnknown StatsMessageClass = 0xFF
)

type StatsMethod stun.Method

const (
	MethodUnknown StatsMethod = 0xFFFF
)

//go:generate stringer -type=TurnFailureReason,RelayDirection -output enums.go
type TurnFailureReason int

const (
	TurnFailureUnknown TurnFailureReason = iota
	TurnFailureAuth
	TurnFailureAllocation
	TurnFailureCreatePermission
	TurnFailureSendIndicaton
	TurnFailureChannelBindRequest
	TurnFailureChannelData
	TurnFailureRefresh
)

type TurnTransport string

const (
	// Only udp is accepted for allocations.
	TurnTransportUDP TurnTransport = "udp"
)

type IPVersion string

const (
	// Allocations are only created for "udp4" networks.
	IPVersion4 IPVersion = "ipv4"
)

type RelayDirection int

const (
	RelayDirectionUnknown RelayDirection = iota
	RelayDirectionClientToServer
	RelayDirectionServerToPeer
	RelayDirectionPeerToServer
	RelayDirectionServerToClient
)

// StatsRecorder defines an interface for recording various runtime metrics
// from a TURN server implementation. This includes counters for STUN/TURN messages,
// allocation lifecycle events, channel data stats, relay usage, and more.
// Each method is meant to be called in response to specific runtime events.
// Implementations can use this interface to expose metrics to Prometheus,
// log data for observability, or plug into external monitoring systems.
//
//go:generate mockgen -source=stats.go -destination=./stats_mock.go -typed -package=stats
type StatsRecorder interface {
	IncTotalMessage(realm string, c StatsMessageClass, m StatsMethod)
	IncFailedMessage(realm string, c StatsMessageClass, s StatsMethod, err error)
	IncTotalChannelData(realm string)
	IncFailedChannelData(realm string, err error)
	IncActiveAllocation(realm string, transport TurnTransport, ipv IPVersion)
	DecActiveAllocation(realm string, transport TurnTransport, ipv IPVersion)
	IncTotalTurn(realm string, msgType stun.MessageType)
	IncFailedTurn(realm string, msgType stun.MessageType, reason TurnFailureReason, err error)
	IncRelayBytes(realm string, n int, dir RelayDirection, transport TurnTransport, ipv IPVersion)
	IncRelayPackets(realm string, dir RelayDirection, transport TurnTransport, ipv IPVersion)
	AddAllocationDuration(realm string, d time.Duration, transport TurnTransport, ipv IPVersion)
}

type NoopStatsRecorder struct{}

func (*NoopStatsRecorder) IncTotalMessage(realm string, c StatsMessageClass, m StatsMethod) {}
func (*NoopStatsRecorder) IncFailedMessage(realm string, c StatsMessageClass, s StatsMethod, err error) {
}
func (*NoopStatsRecorder) IncTotalChannelData(realm string)                                         {}
func (*NoopStatsRecorder) IncFailedChannelData(realm string, err error)                             {}
func (*NoopStatsRecorder) IncActiveAllocation(realm string, transport TurnTransport, ipv IPVersion) {}
func (*NoopStatsRecorder) DecActiveAllocation(realm string, transport TurnTransport, ipv IPVersion) {}
func (*NoopStatsRecorder) IncTotalTurn(realm string, msgType stun.MessageType)                      {}
func (*NoopStatsRecorder) IncFailedTurn(
	realm string,
	msgType stun.MessageType,
	reason TurnFailureReason,
	err error) {
}

func (*NoopStatsRecorder) IncRelayBytes(
	realm string,
	n int,
	dir RelayDirection,
	transport TurnTransport,
	ipv IPVersion) {
}

func (*NoopStatsRecorder) IncRelayPackets(realm string, dir RelayDirection, transport TurnTransport, ipv IPVersion) {
}

func (*NoopStatsRecorder) AddAllocationDuration(realm string, d time.Duration, transport TurnTransport, ipv IPVersion) {
}

type StatsPacketConn struct {
	net.PacketConn
	statsRecorder StatsRecorder
	turnSocket    bool
	realm         string
}

func NewStatsPacketConn(
	c net.PacketConn,
	statsRecorder StatsRecorder,
	realm string,
	turnSocket bool,
) *StatsPacketConn {
	return &StatsPacketConn{
		PacketConn:    c,
		statsRecorder: statsRecorder,
		turnSocket:    turnSocket,
		realm:         realm,
	}
}

func (sconn *StatsPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, addr, err = sconn.PacketConn.ReadFrom(p)

	direction := RelayDirectionPeerToServer
	if sconn.turnSocket {
		direction = RelayDirectionClientToServer
	}

	sconn.statsRecorder.IncRelayBytes(
		sconn.realm,
		n,
		direction,
		TurnTransportUDP,
		IPVersion4,
	)

	sconn.statsRecorder.IncRelayPackets(
		sconn.realm,
		direction,
		TurnTransportUDP,
		IPVersion4,
	)

	return
}

func (sconn *StatsPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	n, err = sconn.PacketConn.WriteTo(p, addr)

	direction := RelayDirectionServerToPeer
	if sconn.turnSocket {
		direction = RelayDirectionServerToClient
	}
	sconn.statsRecorder.IncRelayBytes(
		sconn.realm,
		n,
		direction,
		TurnTransportUDP,
		IPVersion4,
	)
	sconn.statsRecorder.IncRelayPackets(
		sconn.realm,
		direction,
		TurnTransportUDP,
		IPVersion4,
	)

	return
}
