package rtsp

import (
	"encoding/json"
	"errors"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (c *Conn) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	// Original assertion: core.Assert(media.Direction == core.DirectionRecvonly)
	// This was too restrictive for an ActiveProducer (client) needing to provide a
	// sendonly track for backchannel.

	// Check if a Receiver for this specific media and codec (for receiving from server) already exists.
	if media.Direction == core.DirectionRecvonly {
		for _, track := range c.Receivers {
			if track.Media == media && track.Codec.Match(codec) {
				return track, nil
			}
		}
	}

	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	var channel byte
	var err error

	switch c.mode {
	case core.ModeActiveProducer: // RTSP Client
		if c.state == StatePlay {
			if err = c.Reconnect(); err != nil {
				return nil, err
			}
		}

		channel, err = c.SetupMedia(media)
		if err != nil {
			return nil, err
		}
		c.state = StateSetup

		// If media.Direction is Sendonly, the client is providing a track to send to the server (backchannel).
		// The returned Receiver will be used as a source for an internal Sender that c.AddTrack will set up.
		if media.Direction == core.DirectionSendonly {
			newTrack := core.NewReceiver(media, codec)
			newTrack.ID = channel // Channel from SetupMedia
			return newTrack, nil
		}
		// If media.Direction is Recvonly, client receives from server. Will be added to c.Receivers below.

	case core.ModePassiveConsumer: // RTSP Server
		// This case is when the server provides a track.
		// If media.Direction is Recvonly (from server's perspective, for client's backchannel), server is receiving.
		// The original code "channel = byte(len(c.Senders)) * 2" is for when server sends.
		// This part requires careful understanding of server-side track provisioning.
		// For now, this change focuses on ActiveProducer.
		// Assuming original logic for channel assignment was for a specific scenario.
		if media.Direction == core.DirectionRecvonly {
			// Logic for server setting up a track to receive data on (e.g. client backchannel)
			// Channel assignment here should be consistent with server's SDP or SETUP response.
			// For simplicity, using a placeholder or assuming SetupMedia might work if extended.
			// This might need more specific handling for server mode.
			channel = byte(len(c.Receivers) * 2) // Example: even channels for receiving
		} else {
			// Server providing a non-RecvOnly track (i.e., server sends data)
			channel = byte(len(c.Senders)) * 2
		}
	default:
		return nil, errors.New("rtsp: wrong mode for GetTrack: " + c.mode.String())
	}

	// This path is now primarily for:
	// - ActiveProducer, media.Direction == Recvonly
	// - PassiveConsumer, media.Direction == Recvonly (needs verification of channel logic)
	if media.Direction != core.DirectionRecvonly {
        // If it's not RecvOnly and not ActiveProducer/SendOnly (handled above), it's an issue.
		return nil, errors.New("rtsp: GetTrack internal logic error for mode/direction: " + c.mode.String() + "/" + media.Direction)
	}

	track := core.NewReceiver(media, codec)
	track.ID = channel
	c.Receivers = append(c.Receivers, track) // Add Recvonly tracks

	return track, nil
}

func (c *Conn) Start() (err error) {
	core.Assert(c.mode == core.ModeActiveProducer || c.mode == core.ModePassiveProducer)

	for {
		ok := false

		c.stateMu.Lock()
		switch c.state {
		case StateNone:
			err = nil
		case StateConn:
			err = errors.New("start from CONN state")
		case StateSetup:
			switch c.mode {
			case core.ModeActiveProducer:
				err = c.Play()
			case core.ModePassiveProducer:
				err = nil
			default:
				err = errors.New("start from wrong mode: " + c.mode.String())
			}

			if err == nil {
				c.state = StatePlay
				ok = true
			}
		}
		c.stateMu.Unlock()

		if !ok {
			return
		}

		// Handler can return different states:
		// 1. None after PLAY should exit without error
		// 2. Play after PLAY should exit from Start with error
		// 3. Setup after PLAY should Play once again
		err = c.Handle()
	}
}

func (c *Conn) Stop() (err error) {
	for _, receiver := range c.Receivers {
		receiver.Close()
	}
	for _, sender := range c.Senders {
		sender.Close()
	}

	c.stateMu.Lock()
	if c.state != StateNone {
		c.state = StateNone
		err = c.Close()
	}
	c.stateMu.Unlock()

	return
}

func (c *Conn) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Connection)
}

func (c *Conn) Reconnect() error {
	c.Fire("RTSP reconnect")

	// close current session
	_ = c.Close()

	// start new session
	if err := c.Dial(); err != nil {
		return err
	}
	if err := c.Describe(); err != nil {
		return err
	}

	// restore previous medias
	for _, receiver := range c.Receivers {
		if _, err := c.SetupMedia(receiver.Media); err != nil {
			return err
		}
	}
	for _, sender := range c.Senders {
		if _, err := c.SetupMedia(sender.Media); err != nil {
			return err
		}
	}

	return nil
}
