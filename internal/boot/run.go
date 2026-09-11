package boot

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Supervisor lanza la versión vigente y la vigila.
type Supervisor struct {
	Store Store
	// Args son los del servidor: `serve --http … --dir …`, tal cual llegaron.
	Args []string
	// Health es a dónde preguntar si ya está sirviendo.
	Health string
	// Grace es cuánto se le da a una versión nueva para contestar. Pasado eso
	// sin respuesta, se considera que no sirvió.
	Grace time.Duration
	// Settle es cuánto tiene que aguantar contestando antes de llamarla buena.
	// Sin esto, una versión que arranca, contesta una vez y se muere en bucle
	// se marcaría como última-buena y no habría a dónde volver.
	Settle time.Duration
}

// Run no vuelve hasta que alguien pide parar de verdad.
func (s *Supervisor) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	for {
		tag := s.Store.Current()
		if tag == "" {
			return fmt.Errorf("no hay ninguna versión instalada en %s", s.Store.Dir)
		}
		healthy, code, err := s.once(ctx, tag)
		if ctx.Err() != nil {
			return nil // nos pidieron parar; el hijo ya se fue con nosotros
		}
		if err != nil {
			log.Printf("boot: %s no pudo lanzarse: %v", tag, err)
		}

		d := Decide(tag, s.Store.LastGood(), s.Store.Next(), healthy, code)
		log.Printf("boot: %s", d.Why)
		if d.Tag != tag {
			if err := s.Store.SetCurrent(d.Tag); err != nil {
				return err
			}
		}
		// La petición se consume pase lo que pase: una que sobrevive al intento
		// reintentaría la misma versión rota en cada arranque, para siempre.
		if s.Store.Next() != "" {
			_ = s.Store.ClearNext()
		}
		if d.Rollback {
			// Un respiro antes de relanzar, para no gastar el disco en logs si
			// lo que falla es el entorno y no la versión.
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
		}
	}
}

// once lanza una versión y espera a que termine. Devuelve si llegó a servir.
func (s *Supervisor) once(ctx context.Context, tag string) (healthy bool, code int, err error) {
	cmd := exec.Command(s.Store.Binary(tag), s.Args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	cmd.Env = append(os.Environ(), ChildEnv+"=1")
	if err := cmd.Start(); err != nil {
		return false, 0, err
	}
	log.Printf("boot: sirviendo %s (pid %d)", tag, cmd.Process.Pid)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Se le pasa la señal al hijo y se espera: matarlo de golpe dejaría la base
	// a medio cerrar.
	go func() {
		<-ctx.Done()
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
	}()

	good := make(chan struct{})
	go func() {
		if s.await(ctx, cmd) {
			close(good)
		}
	}()

	select {
	case <-good:
		healthy = true
		if s.Store.LastGood() != tag {
			if err := s.Store.SetLastGood(tag); err != nil {
				log.Printf("boot: no pude anotar %s como buena: %v", tag, err)
			} else {
				log.Printf("boot: %s aguantó %s contestando; queda como última buena", tag, s.Settle)
			}
		}
		err = <-done
	case err = <-done:
	}
	return healthy, exitCode(err), err
}

// await dice si la versión llegó a servir Y se sostuvo.
func (s *Supervisor) await(ctx context.Context, cmd *exec.Cmd) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(s.Grace)
	var since time.Time
	for time.Now().Before(deadline) || !since.IsZero() {
		if ctx.Err() != nil {
			return false
		}
		if alive(client, s.Health) {
			if since.IsZero() {
				since = time.Now()
			}
			if time.Since(since) >= s.Settle {
				return true
			}
		} else {
			// Contestó y dejó de hacerlo: el reloj de asentarse vuelve a cero.
			since = time.Time{}
			if time.Now().After(deadline) {
				return false
			}
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(time.Second):
		}
	}
	return false
}

func alive(c *http.Client, url string) bool {
	res, err := c.Get(url)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode == http.StatusOK
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if ok := asExit(err, &ee); ok {
		return ee.ExitCode()
	}
	return -1
}

func asExit(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

// HealthURL deduce a dónde preguntar a partir de la dirección de escucha.
//
// `0.0.0.0` y `::` son «en todas», no una dirección a la que se pueda llamar:
// se pregunta por el loopback, que es donde siempre está.
func HealthURL(addr string) string {
	if addr == "" {
		addr = "127.0.0.1:8090"
	}
	host, port, ok := strings.Cut(addr, ":")
	if !ok {
		host, port = "127.0.0.1", addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + host + ":" + port + "/api/version"
}
