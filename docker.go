package docker

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/replacer"
	"github.com/coredns/coredns/request"

	"github.com/caddyserver/caddy"
	"github.com/miekg/dns"

	docker "github.com/fsouza/go-dockerclient"
)

// Docker implement the plugin interface.
type Docker struct {
	Next plugin.Handler
}

func init() { plugin.Register("docker", setup) }

func setup(c *caddy.Controller) error {
	for c.Next() {
		if c.NextArg() {
			return plugin.Error("docker", c.ArgErr())
		}
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return Docker{Next: next}
	})

	return nil
}

const format = `{remote} ` + replacer.EmptyValue + ` {>id} {type} {class} {name} {proto} {port}`

var output io.Writer = os.Stdout

// ServeDNS implements the plugin.Handler interface.
func (d Docker) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	const endpoint = "unix:///var/run/docker.sock"

	state := request.Request{W: w, Req: r}
	/*
		qname := state.Name()
		qtype := state.Type()
	*/
	path := os.Getenv("DOCKER_CERT_PATH")
	ca := fmt.Sprintf("%s/ca.pem", path)
	cert := fmt.Sprintf("%s/cert.pem", path)
	key := fmt.Sprintf("%s/key.pem", path)
	client, err := docker.NewTLSClient(endpoint, cert, key, ca)
	//rrw := dnstest.NewRecorder(w)
	//rep := replacer.New()
	//fmt.Fprintln(output, rep.Replace(r, rrw, replacer.EmptyValue, format))

	if err != nil {
		panic(err)
	}
	imgs, err := client.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		panic(err)
	}
	eventChan := make(chan *docker.APIEvents, 100)
	err_de := client.AddEventListener(eventChan)
	if err_de != nil {
		fmt.Printf("Error registering docker event listener: %s", err_de)
	}

	go func() {

		for {
			if client == nil {
				break
			}
			/*	if !watching {
				err := client.AddEventListener(eventChan)
				if err != nil && err != docker.ErrListenerAlreadyExists {
					log.Printf("Error registering docker event listener: %s", err)
					time.Sleep(10 * time.Second)
					continue
				}
				watching = true
				log.Println("Watching docker events")
				// sync all configs after resuming listener
				g.generateFromContainers()
			}*/
			select {
			case event, ok := <-eventChan:
				if !ok {
					/*	log.Printf("Docker daemon connection interrupted")
						if watching {
							client.RemoveEventListener(eventChan)
							watching = false
							client = nil
						}
						if !g.retry {
							// close all watchers and exit
							for _, watcher := range watchers {
								close(watcher)
							}
							return
						}*/
					// recreate channel and attempt to resume
					//eventChan = make(chan *docker.APIEvents, 100)
					//	time.Sleep(10 * time.Second)
					break
				}
				fmt.Fprintln(output, event.Status)
				if event.Status == "start" || event.Status == "stop" || event.Status == "die" {
					fmt.Printf("Received event %s for container %s", event.Status, event.ID[:12])
					fmt.Fprintln(output, "rwaDDDDDDDDDDr")
					// fanout event to all watchers
					/*	for _, watcher := range watchers {
						watcher <- event
					}*/
				}
				/*case <-time.After(10 * time.Second):
				// check for docker liveness
				err := client.Ping()
				if err != nil {
					log.Printf("Unable to ping docker daemon: %s", err)
					if watching {
						client.RemoveEventListener(eventChan)
						watching = false
						client = nil
					}
				}*/
				/*	case sig := <-sigChan:
					log.Printf("Received signal: %s\n", sig)
					switch sig {
					case syscall.SIGQUIT, syscall.SIGKILL, syscall.SIGTERM, syscall.SIGINT:
						// close all watchers and exit
						for _, watcher := range watchers {
							close(watcher)
						}
						return
					}*/
			}
		}
	}()

	for _, img := range imgs {
		fmt.Println("ID: ", img.ID)
		container, _ := client.InspectContainer(img.ID)
		for _, ct := range container.Config.Env {
			fmt.Println("ID: ", ct)
		}
	}
	answers := make([]dns.RR, 0, 10)
	extras := make([]dns.RR, 0, 10)
	/*
		r := new(dns.A)
		r.Hdr = dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeA,
			Class: dns.ClassINET, Ttl: redis.minTtl(a.Ttl)}
		r.A = a.Ip

		https://github.com/arvancloud/redis/blob/8a6e3c44c0ac1ae258addbe741365c2b69992350/redis.go
	*/

	t := new(dns.A)
	t.Hdr = dns.RR_Header{Name: "www.example.org.", Rrtype: dns.TypeA,
		Class: dns.ClassINET, Ttl: 120}
	t.A = net.IPv4(127, 0, 0, 42)

	answers = append(answers, t)

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, false, true

	m.Answer = append(m.Answer, answers...)
	m.Extra = append(m.Extra, extras...)

	state.SizeAndDo(m)
	m = state.Scrub(m)

	_ = w.WriteMsg(m)

	return dns.RcodeSuccess, nil
	//return plugin.NextOrFailure(d.Name(), d.Next, ctx, w, r)
}

// Name implements the Handler interface.
func (d Docker) Name() string { return "docker" }
