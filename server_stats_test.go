// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package turn

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/pion/transport/v3/test"
	"github.com/pion/turn/v4/internal/proto"
	"github.com/pion/turn/v4/stats"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestServerStats(t *testing.T) { //nolint
	lim := test.TimeOut(time.Second * 30)
	defer lim.Stop()

	report := test.CheckRoutines(t)
	defer report()

	loggerFactory := logging.NewDefaultLoggerFactory()

	credMap := map[string][]byte{
		"user": GenerateAuthKey("user", "pion.ly", "pass"),
	}

	t.Run("binding request", func(t *testing.T) {
		udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
		assert.NoError(t, err)

		ctrl := gomock.NewController(t)
		statsMock := stats.NewMockStatsRecorder(ctrl)

		server, err := NewServer(ServerConfig{
			AuthHandler: func(username, _ string, _ net.Addr) (key []byte, ok bool) {
				if pw, ok := credMap[username]; ok {
					return pw, true
				}

				return nil, false
			},
			PacketConnConfigs: []PacketConnConfig{
				{
					PacketConn: udpListener,
					RelayAddressGenerator: &RelayAddressGeneratorStatic{
						RelayAddress: net.ParseIP("127.0.0.1"),
						Address:      "0.0.0.0",
					},
				},
			},
			Realm:         "pion.ly",
			LoggerFactory: loggerFactory,
			StatsRecoder:  statsMock,
		})
		assert.NoError(t, err)

		assert.Equal(t, proto.DefaultLifetime, server.channelBindTimeout, "should match")

		conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
		assert.NoError(t, err)

		statsMock.EXPECT().IncRelayBytes(
			gomock.Eq("pion.ly"),
			gomock.Any(),
			gomock.AnyOf(stats.RelayDirectionClientToServer, stats.RelayDirectionServerToClient),
			gomock.Eq(stats.TurnTransportUDP),
			gomock.Eq(stats.IPVersion4),
		).Times(2)

		statsMock.EXPECT().IncRelayPackets(
			gomock.Eq("pion.ly"),
			gomock.AnyOf(stats.RelayDirectionClientToServer, stats.RelayDirectionServerToClient),
			gomock.Eq(stats.TurnTransportUDP),
			gomock.Eq(stats.IPVersion4),
		).Times(2)

		statsMock.EXPECT().IncTotalMessage(
			gomock.Eq("pion.ly"),
			gomock.Eq(stats.StatsMessageClass(stun.ClassRequest)),
			gomock.Eq(stats.StatsMethod(stun.MethodBinding)),
		).Times(1)

		client, err := NewClient(&ClientConfig{
			Conn:          conn,
			LoggerFactory: loggerFactory,
		})
		assert.NoError(t, err)
		assert.NoError(t, client.Listen())

		_, err = client.SendBindingRequestTo(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3478})
		assert.NoError(t, err, "should succeed")

		client.Close()
		assert.NoError(t, conn.Close())

		assert.NoError(t, server.Close())
	})

	t.Run("allocation request", func(t *testing.T) {
		udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
		assert.NoError(t, err)

		ctrl := gomock.NewController(t)
		statsMock := stats.NewMockStatsRecorder(ctrl)

		server, err := NewServer(ServerConfig{
			AuthHandler: func(username, _ string, _ net.Addr) (key []byte, ok bool) {
				if pw, ok := credMap[username]; ok {
					return pw, true
				}

				return nil, false
			},
			PacketConnConfigs: []PacketConnConfig{
				{
					PacketConn: udpListener,
					RelayAddressGenerator: &RelayAddressGeneratorStatic{
						RelayAddress: net.ParseIP("127.0.0.1"),
						Address:      "0.0.0.0",
					},
				},
			},
			Realm:         "pion.ly",
			LoggerFactory: loggerFactory,
			StatsRecoder:  statsMock,
		})
		assert.NoError(t, err)

		assert.Equal(t, proto.DefaultLifetime, server.channelBindTimeout, "should match")

		conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
		assert.NoError(t, err)

		addr := "127.0.0.1:3478"
		client, err := NewClient(&ClientConfig{
			STUNServerAddr: addr,
			TURNServerAddr: addr,
			Conn:           conn,
			LoggerFactory:  loggerFactory,
			Realm:          "pion.ly",
			Username:       "user",
			Password:       "pass",
		})

		assert.NoError(t, err)
		assert.NoError(t, client.Listen())

		prepareForAllocationRequest(statsMock)

		relayConn, err := client.Allocate()
		assert.NoError(t, err, "should succeed")

		assert.True(t, ctrl.Satisfied(), "Prior function calls must be invoked")

		prepareForAllocationClose(statsMock)
		time.Sleep(200 * time.Millisecond)
		assert.NoError(t, relayConn.Close())

		client.Close()
		assert.NoError(t, conn.Close())

		assert.NoError(t, server.Close())

		waitForCtrl(ctrl, time.Second)
	})

	t.Run("create permission", func(t *testing.T) {
		udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
		assert.NoError(t, err)

		ctrl := gomock.NewController(t)
		statsMock := stats.NewMockStatsRecorder(ctrl)

		server, err := NewServer(ServerConfig{
			AuthHandler: func(username, _ string, _ net.Addr) (key []byte, ok bool) {
				if pw, ok := credMap[username]; ok {
					return pw, true
				}

				return nil, false
			},
			PacketConnConfigs: []PacketConnConfig{
				{
					PacketConn: udpListener,
					RelayAddressGenerator: &RelayAddressGeneratorStatic{
						RelayAddress: net.ParseIP("127.0.0.1"),
						Address:      "0.0.0.0",
					},

					PermissionHandler: func(src net.Addr, peer net.IP) bool {
						return src.String() == "127.0.0.1:54321" &&
							peer.Equal(net.ParseIP("127.0.0.4"))
					},
				},
			},

			Realm:         "pion.ly",
			LoggerFactory: loggerFactory,
			StatsRecoder:  statsMock,
		})
		assert.NoError(t, err)

		assert.Equal(t, proto.DefaultLifetime, server.channelBindTimeout, "should match")

		// Enforce correct client IP and port
		conn, err := net.ListenPacket("udp4", "127.0.0.1:54321")
		assert.NoError(t, err)

		addr := "127.0.0.1:3478"
		client, err := NewClient(&ClientConfig{
			STUNServerAddr: addr,
			TURNServerAddr: addr,
			Conn:           conn,
			LoggerFactory:  loggerFactory,
			Realm:          "pion.ly",
			Username:       "user",
			Password:       "pass",
		})

		assert.NoError(t, err)
		assert.NoError(t, client.Listen())

		prepareForAllocationRequest(statsMock)

		relayConn, err := client.Allocate()
		assert.NoError(t, err, "should succeed")
		assert.True(t, ctrl.Satisfied(), "Prior function calls must be invoked")

		whiteAddr, errA := net.ResolveUDPAddr("udp", "127.0.0.4:12345")
		assert.NoError(t, errA, "should succeed")
		blackAddr, errB1 := net.ResolveUDPAddr("udp", "127.0.0.5:12345")
		assert.NoError(t, errB1, "should succeed")

		prepareForCreatePermission(statsMock, true)
		err = client.CreatePermission(whiteAddr)
		assert.NoError(t, err, "grant permission for whitelisted peer")
		assert.True(t, ctrl.Satisfied(), "Prior function calls must be invoked")

		prepareForCreatePermission(statsMock, false)
		err = client.CreatePermission(blackAddr)
		assert.ErrorContains(t, err, "error", "deny permission for blacklisted peer address")
		assert.True(t, ctrl.Satisfied(), "Prior function calls must be invoked")

		prepareForCreatePermission(statsMock, true)
		err = client.CreatePermission(whiteAddr, whiteAddr)
		assert.NoError(t, err, "grant permission for repeated whitelisted peer addresses")
		assert.True(t, ctrl.Satisfied(), "Prior function calls must be invoked")

		prepareForCreatePermission(statsMock, false)
		err = client.CreatePermission(blackAddr)
		assert.ErrorContains(t, err, "error", "deny permission for repeated blacklisted peer address")
		assert.True(t, ctrl.Satisfied(), "Prior function calls must be invoked")

		// Isn't this a corner case in the spec?
		prepareForCreatePermission(statsMock, false)
		err = client.CreatePermission(whiteAddr, blackAddr)
		assert.ErrorContains(t, err, "error", "deny permission for mixed whitelisted and blacklisted peers")
		assert.True(t, ctrl.Satisfied(), "Prior function calls must be invoked")

		prepareForAllocationClose(statsMock)
		time.Sleep(200 * time.Millisecond)
		assert.NoError(t, relayConn.Close())

		client.Close()
		assert.NoError(t, conn.Close())

		assert.NoError(t, server.Close())

		waitForCtrl(ctrl, time.Second)
	})

	t.Run("send and recv", func(t *testing.T) {
		assert.FailNow(t, "not tested")
	})

	t.Run("send and recv with perms", func(t *testing.T) {
		assert.FailNow(t, "not tested")
	})

	t.Run("send and recv with channel bind", func(t *testing.T) {
		assert.FailNow(t, "not tested")
	})

	t.Run("refresh", func(t *testing.T) {
		assert.FailNow(t, "not tested")
	})

	t.Run("auth failure", func(t *testing.T) {
		assert.FailNow(t, "not tested")
	})
}

func prepareForAllocationRequest(statsMock *stats.MockStatsRecorder) {
	statsMock.EXPECT().IncTotalTurn(
		gomock.Eq("pion.ly"),
		gomock.Eq(stun.MessageType{
			Class:  stun.ClassRequest,
			Method: stun.MethodAllocate,
		}),
	).Times(2)

	statsMock.EXPECT().IncFailedTurn(
		gomock.Eq("pion.ly"),
		gomock.Eq(stun.MessageType{
			Class:  stun.ClassRequest,
			Method: stun.MethodAllocate,
		}),
		gomock.Eq(stats.TurnFailureAuth),
		gomock.Any(),
	).Times(1)

	statsMock.EXPECT().IncRelayBytes(
		gomock.Eq("pion.ly"),
		gomock.Any(),
		gomock.AnyOf(stats.RelayDirectionClientToServer, stats.RelayDirectionServerToClient),
		gomock.Eq(stats.TurnTransportUDP),
		gomock.Eq(stats.IPVersion4),
	).Times(4)

	statsMock.EXPECT().IncRelayPackets(
		gomock.Eq("pion.ly"),
		gomock.AnyOf(stats.RelayDirectionClientToServer, stats.RelayDirectionServerToClient),
		gomock.Eq(stats.TurnTransportUDP),
		gomock.Eq(stats.IPVersion4),
	).Times(4)

	statsMock.EXPECT().IncTotalMessage(
		gomock.Eq("pion.ly"),
		gomock.Eq(stats.StatsMessageClass(stun.ClassRequest)),
		gomock.Eq(stats.StatsMethod(stun.MethodAllocate)),
	).Times(2)

	statsMock.EXPECT().IncActiveAllocation(
		gomock.Eq("pion.ly"),
		gomock.Eq(stats.TurnTransportUDP),
		gomock.Eq(stats.IPVersion4),
	).Times(1)
}

func prepareForAllocationClose(statsMock *stats.MockStatsRecorder) {
	statsMock.EXPECT().IncTotalTurn(
		gomock.Eq("pion.ly"),
		gomock.Eq(stun.MessageType{
			Class:  stun.ClassRequest,
			Method: stun.MethodRefresh,
		}),
	).Times(1)

	statsMock.EXPECT().IncRelayBytes(
		gomock.Eq("pion.ly"),
		gomock.Any(),
		gomock.AnyOf(stats.RelayDirectionClientToServer, stats.RelayDirectionServerToClient),
		gomock.Eq(stats.TurnTransportUDP),
		gomock.Eq(stats.IPVersion4),
	).Times(2)

	statsMock.EXPECT().IncRelayPackets(
		gomock.Eq("pion.ly"),
		gomock.AnyOf(stats.RelayDirectionClientToServer, stats.RelayDirectionServerToClient),
		gomock.Eq(stats.TurnTransportUDP),
		gomock.Eq(stats.IPVersion4),
	).Times(2)

	statsMock.EXPECT().IncTotalMessage(
		gomock.Eq("pion.ly"),
		gomock.Eq(stats.StatsMessageClass(stun.ClassRequest)),
		gomock.Eq(stats.StatsMethod(stun.MethodRefresh)),
	).Times(1)

	statsMock.EXPECT().DecActiveAllocation(
		gomock.Eq("pion.ly"),
		gomock.Eq(stats.TurnTransportUDP),
		gomock.Eq(stats.IPVersion4),
	).Times(1)

	statsMock.EXPECT().AddAllocationDuration(
		gomock.Eq("pion.ly"),
		gomock.Any(),
		gomock.Eq(stats.TurnTransportUDP),
		gomock.Eq(stats.IPVersion4),
	).Times(1)
}

func prepareForCreatePermission(statsMock *stats.MockStatsRecorder, grant bool) {
	statsMock.EXPECT().IncTotalTurn(
		gomock.Eq("pion.ly"),
		gomock.Eq(stun.MessageType{
			Class:  stun.ClassRequest,
			Method: stun.MethodCreatePermission,
		}),
	).Times(1)

	statsMock.EXPECT().IncTotalMessage(
		gomock.Eq("pion.ly"),
		gomock.Eq(stats.StatsMessageClass(stun.ClassRequest)),
		gomock.Eq(stats.StatsMethod(stun.MethodCreatePermission)),
	).Times(1)

	statsMock.EXPECT().IncRelayBytes(
		gomock.Eq("pion.ly"),
		gomock.Any(),
		gomock.AnyOf(stats.RelayDirectionClientToServer, stats.RelayDirectionServerToClient),
		gomock.Eq(stats.TurnTransportUDP),
		gomock.Eq(stats.IPVersion4),
	).Times(2)

	statsMock.EXPECT().IncRelayPackets(
		gomock.Eq("pion.ly"),
		gomock.AnyOf(stats.RelayDirectionClientToServer, stats.RelayDirectionServerToClient),
		gomock.Eq(stats.TurnTransportUDP),
		gomock.Eq(stats.IPVersion4),
	).Times(2)

	if !grant {
		statsMock.EXPECT().IncFailedTurn(
			gomock.Eq("pion.ly"),
			gomock.Eq(stun.MessageType{
				Class:  stun.ClassRequest,
				Method: stun.MethodCreatePermission,
			}),
			gomock.Eq(stats.TurnFailureCreatePermission),
			gomock.Any(),
		).Times(1)
	}
}

func waitForCtrl(ctrl *gomock.Controller, d time.Duration) {
	timer := time.NewTimer(d)
	for {
		select {
		case <-timer.C:
			return
		default:
			if ctrl.Satisfied() {
				timer.Stop()

				return
			}
		}
	}
}
