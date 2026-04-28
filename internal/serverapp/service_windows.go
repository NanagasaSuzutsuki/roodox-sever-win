//go:build windows

package serverapp

import (
	"log"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc"

	"roodox_server/internal/appconfig"
)

type windowsServiceProgram struct {
	cfg appconfig.Config
}

func RunWindowsService(serviceName string, cfg appconfig.Config) error {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		serviceName = cfg.Runtime.WindowsService.Name
	}
	if serviceName == "" {
		serviceName = "RoodoxServer"
	}
	return svc.Run(serviceName, &windowsServiceProgram{cfg: cfg})
}

func (p *windowsServiceProgram) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}

	rt, err := Start(p.cfg)
	if err != nil {
		log.Printf("component=service op=start error=%q", err.Error())
		changes <- svc.Status{State: svc.Stopped}
		return false, 1
	}

	waitHint := p.cfg.Runtime.GracefulStopTimeoutSeconds
	if waitHint <= 0 {
		waitHint = 10
	}
	accepted := svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{
		State:    svc.Running,
		Accepts:  accepted,
		WaitHint: uint32(time.Duration(waitHint) * time.Second / time.Millisecond),
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- rt.Wait()
	}()

	for {
		select {
		case change := <-requests:
			switch change.Cmd {
			case svc.Interrogate:
				changes <- change.CurrentStatus
			case svc.Stop, svc.Shutdown:
				log.Printf("component=service op=control cmd=%d", change.Cmd)
				changes <- svc.Status{
					State:    svc.StopPending,
					WaitHint: uint32(time.Duration(waitHint) * time.Second / time.Millisecond),
				}
				rt.Stop()
				if err := <-waitCh; err != nil {
					log.Printf("component=service op=stop error=%q", err.Error())
					return false, 1
				}
				return false, 0
			}
		case err := <-waitCh:
			if err != nil {
				log.Printf("component=service op=runtime_exit error=%q", err.Error())
				return false, 1
			}
			return false, 0
		}
	}
}
