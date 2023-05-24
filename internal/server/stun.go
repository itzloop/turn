// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package server

import (
	"github.com/itzloop/turn/v2/internal/ipnet"
	"github.com/pion/stun"
)

func handleBindingRequest(r Request, m *stun.Message) error {
	r.Log.Debugf("received BindingRequest from %s", r.SrcAddr.String())

	ip, port, err := ipnet.AddrIPPort(r.SrcAddr)
	if err != nil {
		return err
	}

	attrs := buildMsg(m.TransactionID, stun.BindingSuccess, &stun.XORMappedAddress{
		IP:   ip,
		Port: port,
	}, stun.Fingerprint)

	return buildAndSend(r.Conn, r.SrcAddr, attrs...)
}
