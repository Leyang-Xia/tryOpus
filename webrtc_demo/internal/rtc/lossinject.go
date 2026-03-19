package rtc

import (
	"io"
	"math/rand"
	"sync"

	"github.com/pion/interceptor"
)

type ReceiverLossConfig struct {
	Uniform    float64
	UseGE      bool
	PGoodToBad float64
	PBadToGood float64
	LossInBad  float64
	Seed       int64
	OnDrop     func()
}

func (c ReceiverLossConfig) Enabled() bool {
	return c.Uniform > 0 || c.UseGE
}

type lossSimulator struct {
	rng        *rand.Rand
	uniform    float64
	useGE      bool
	pGoodToBad float64
	pBadToGood float64
	lossInBad  float64
	inBad      bool
}

func newLossSimulator(seed int64, uniform float64, useGE bool, pGoodToBad float64, pBadToGood float64, lossInBad float64) *lossSimulator {
	return &lossSimulator{
		rng:        rand.New(rand.NewSource(seed)),
		uniform:    uniform,
		useGE:      useGE,
		pGoodToBad: pGoodToBad,
		pBadToGood: pBadToGood,
		lossInBad:  lossInBad,
	}
}

func (s *lossSimulator) shouldDrop() bool {
	if s.useGE {
		if s.inBad {
			if s.rng.Float64() < s.pBadToGood {
				s.inBad = false
			}
		} else if s.rng.Float64() < s.pGoodToBad {
			s.inBad = true
		}
		if s.inBad {
			return s.rng.Float64() < s.lossInBad
		}
		return false
	}
	if s.uniform <= 0 {
		return false
	}
	return s.rng.Float64() < s.uniform
}

type lossInjectorFactory struct {
	cfg ReceiverLossConfig
}

func newLossInjectorFactory(cfg ReceiverLossConfig) interceptor.Factory {
	return &lossInjectorFactory{cfg: cfg}
}

func (f *lossInjectorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	return &lossInjector{
		cfg:        f.cfg,
		simulators: map[uint32]*lossSimulator{},
	}, nil
}

type lossInjector struct {
	cfg        ReceiverLossConfig
	mu         sync.Mutex
	simulators map[uint32]*lossSimulator
}

func (l *lossInjector) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return reader
}

func (l *lossInjector) BindRTCPWriter(writer interceptor.RTCPWriter) interceptor.RTCPWriter {
	return writer
}

func (l *lossInjector) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	return writer
}

func (l *lossInjector) UnbindLocalStream(info *interceptor.StreamInfo) {}

func (l *lossInjector) simulatorFor(ssrc uint32) *lossSimulator {
	l.mu.Lock()
	defer l.mu.Unlock()
	if sim, ok := l.simulators[ssrc]; ok {
		return sim
	}
	sim := newLossSimulator(
		l.cfg.Seed+int64(ssrc),
		l.cfg.Uniform,
		l.cfg.UseGE,
		l.cfg.PGoodToBad,
		l.cfg.PBadToGood,
		l.cfg.LossInBad,
	)
	l.simulators[ssrc] = sim
	return sim
}

func (l *lossInjector) BindRemoteStream(info *interceptor.StreamInfo, reader interceptor.RTPReader) interceptor.RTPReader {
	sim := l.simulatorFor(info.SSRC)
	return interceptor.RTPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		for {
			n, attr, err := reader.Read(b, a)
			if err != nil {
				return n, attr, err
			}
			if !sim.shouldDrop() {
				return n, attr, nil
			}
			if l.cfg.OnDrop != nil {
				l.cfg.OnDrop()
			}
		}
	})
}

func (l *lossInjector) UnbindRemoteStream(info *interceptor.StreamInfo) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.simulators, info.SSRC)
}

func (l *lossInjector) Close() error {
	return nil
}

func WithReceiverLossSimulation(cfg ReceiverLossConfig) PeerOption {
	return func(params *peerOptions) error {
		if cfg.Enabled() {
			params.receiverLoss = &cfg
		}
		return nil
	}
}

var _ interceptor.Interceptor = (*lossInjector)(nil)
var _ io.Closer = (*lossInjector)(nil)
