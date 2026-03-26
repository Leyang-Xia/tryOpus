package rtc

import (
	"fmt"

	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
)

type peerOptions struct {
	receiverLoss *ReceiverLossConfig
}

type PeerOption func(*peerOptions) error

func NewPeerConnection(config webrtc.Configuration, opts ...PeerOption) (*webrtc.PeerConnection, error) {
	params := peerOptions{}
	for _, opt := range opts {
		if err := opt(&params); err != nil {
			return nil, err
		}
	}

	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("register default codecs failed: %w", err)
	}

	interceptorRegistry := &interceptor.Registry{}
	if err := webrtc.ConfigureTWCCHeaderExtensionSender(mediaEngine, interceptorRegistry); err != nil {
		return nil, fmt.Errorf("configure twcc header extension failed: %w", err)
	}
	if params.receiverLoss != nil {
		interceptorRegistry.Add(newLossInjectorFactory(*params.receiverLoss))
	}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		return nil, fmt.Errorf("register default interceptors failed: %w", err)
	}

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
	)
	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("create peer connection failed: %w", err)
	}
	return pc, nil
}
