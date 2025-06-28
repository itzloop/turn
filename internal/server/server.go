// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package server implements the private API to implement a TURN server
package server

import (
	"fmt"
	"net"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/pion/turn/v4/internal/allocation"
	"github.com/pion/turn/v4/internal/proto"
	"github.com/pion/turn/v4/stats"
)

// Request contains all the state needed to process a single incoming datagram.
type Request struct {
	// Current Request State
	Conn    net.PacketConn
	SrcAddr net.Addr
	Buff    []byte

	// Server State
	AllocationManager *allocation.Manager
	NonceHash         *NonceHash

	// User Configuration
	AuthHandler        func(username string, realm string, srcAddr net.Addr) (key []byte, ok bool)
	Log                logging.LeveledLogger
	Realm              string
	ChannelBindTimeout time.Duration

	StatsRecorder stats.StatsRecorder
}

// HandleRequest processes the give Request.
func HandleRequest(r Request) error {
	r.Log.Debugf("Received %d bytes of udp from %s on %s", len(r.Buff), r.SrcAddr, r.Conn.LocalAddr())

	if proto.IsChannelData(r.Buff) {
		return handleDataPacket(r)
	}

	return handleTURNPacket(r)
}

func handleDataPacket(req Request) error {
	req.Log.Debugf("Received DataPacket from %s", req.SrcAddr.String())
	req.StatsRecorder.IncTotalChannelData(req.Realm)
	c := proto.ChannelData{Raw: req.Buff}
	if err := c.Decode(); err != nil {
		// TODO: more data for ChannelData?
		err = fmt.Errorf("%w: %v", errFailedToCreateChannelData, err) //nolint:errorlint
		req.StatsRecorder.IncFailedChannelData(req.Realm, err)

		return err
	}

	err := handleChannelData(req, &c)
	if err != nil {
		err = fmt.Errorf("%w from %v: %v", errUnableToHandleChannelData, req.SrcAddr, err) //nolint:errorlint
		req.StatsRecorder.IncFailedChannelData(req.Realm, err)
	}

	return err
}

func handleTURNPacket(req Request) error {
	req.Log.Debug("Handling TURN packet")
	stunMsg := &stun.Message{Raw: append([]byte{}, req.Buff...)}
	if err := stunMsg.Decode(); err != nil {
		// nolint:errorlint
		req.StatsRecorder.IncTotalMessage(req.Realm, stats.ClassUnknown, stats.MethodUnknown)
		err = fmt.Errorf("%w: %v", errFailedToCreateSTUNPacket, err)
		req.StatsRecorder.IncFailedMessage(req.Realm, stats.ClassUnknown, stats.MethodUnknown, err)

		return err
	}

	req.StatsRecorder.IncTotalMessage(
		req.Realm,
		stats.StatsMessageClass(stunMsg.Type.Class),
		stats.StatsMethod(stunMsg.Type.Method),
	)

	handler, err := getMessageHandler(stunMsg.Type.Class, stunMsg.Type.Method)
	if err != nil {
		// nolint:errorlint
		err = fmt.Errorf(
			"%w %v-%v from %v: %v",
			errUnhandledSTUNPacket,
			stunMsg.Type.Method,
			stunMsg.Type.Class,
			req.SrcAddr,
			err,
		)

		req.StatsRecorder.IncFailedMessage(
			req.Realm,
			stats.StatsMessageClass(stunMsg.Type.Class),
			stats.StatsMethod(stunMsg.Type.Method),
			err,
		)

		return err
	}

	err = handler(req, stunMsg)
	if err != nil {
		// nolint:errorlint
		err = fmt.Errorf(
			"%w %v-%v from %v: %v",
			errFailedToHandle,
			stunMsg.Type.Method,
			stunMsg.Type.Class,
			req.SrcAddr,
			err,
		)

		req.StatsRecorder.IncFailedMessage(
			req.Realm,
			stats.StatsMessageClass(stunMsg.Type.Class),
			stats.StatsMethod(stunMsg.Type.Method),
			err,
		)

		return err
	}

	return nil
}

func getMessageHandler(class stun.MessageClass, method stun.Method) ( // nolint:cyclop
	func(req Request, stunMsg *stun.Message) error,
	error,
) {
	switch class {
	case stun.ClassIndication:
		switch method {
		case stun.MethodSend:
			return handleSendIndication, nil
		default:
			return nil, fmt.Errorf("%w: %s", errUnexpectedMethod, method)
		}

	case stun.ClassRequest:
		switch method {
		case stun.MethodAllocate:
			return handleAllocateRequest, nil
		case stun.MethodRefresh:
			return handleRefreshRequest, nil
		case stun.MethodCreatePermission:
			return handleCreatePermissionRequest, nil
		case stun.MethodChannelBind:
			return handleChannelBindRequest, nil
		case stun.MethodBinding:
			return handleBindingRequest, nil
		default:
			return nil, fmt.Errorf("%w: %s", errUnexpectedMethod, method)
		}

	default:
		return nil, fmt.Errorf("%w: %s", errUnexpectedClass, class)
	}
}
