package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	pipyRepoAddr    string
	defaultNodePort string
	sdPort          string
)

type ServiceDiscover struct {
	RepoAddr string
	Client   *http.Client
	Repos    map[string][]string
	Logger   zerolog.Logger
}

type InstanceStatus struct {
	IP string `json:"ip"`
}

type RepoInfo struct {
	Instances map[string]InstanceStatus
}

type PromTarget struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

func (sd *ServiceDiscover) GetInstances(repo string) []string {
	instances := []string{}

	resp, err := sd.Client.Get(fmt.Sprintf("%s%s", sd.RepoAddr, repo))
	if err != nil {
		log.Error().Err(err).Msg(fmt.Sprintf("request repo %s failed", repo))
		return instances
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var ri RepoInfo

	err = json.Unmarshal(body, &ri)
	if err != nil {
		log.Error().Err(err).Msg(fmt.Sprintf("decode repo %s data failed", repo))
		return instances
	}

	isExists := map[string]bool{}
	for _, status := range ri.Instances {
		ipAddr, _ := netip.ParseAddr(status.IP)
		if ipAddr.Is6() {
			if ok := isExists[status.IP]; !ok {
				instances = append(instances, fmt.Sprintf("[%s]:%s", status.IP, defaultNodePort))
				isExists[status.IP] = true
			}
		} else {
			if ok := isExists[status.IP]; !ok {
				instances = append(instances, fmt.Sprintf("%s:%s", status.IP, defaultNodePort))
				isExists[status.IP] = true
			}
		}
	}

	return instances
}

func (sd *ServiceDiscover) Update() {
	resp, err := sd.Client.Get(sd.RepoAddr)
	if err != nil {
		log.Error().Err(err).Msg("request repo list failed")
		return
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	repoList := strings.Split(string(body), "\n")
	repoInfo := make(map[string][]string)
	for _, repo := range repoList {
		instances := sd.GetInstances(repo)
		if len(instances) == 0 {
			continue
		}
		repoInfo[repo] = instances
	}

	sd.Repos = repoInfo
}

func (sd *ServiceDiscover) GetPromTargets() []PromTarget {
	targets := make([]PromTarget, 0)

	for repo, instances := range sd.Repos {
		pt := PromTarget{}
		pt.Targets = instances
		pt.Labels = map[string]string{
			"repo": repo,
		}

		targets = append(targets, pt)
	}
	return targets
}

func main() {
	flag.StringVar(&pipyRepoAddr, "repo", "", "pipy repo addr")
	flag.StringVar(&defaultNodePort, "node-port", "9100", "the port of node exporter")
	flag.StringVar(&sdPort, "port", "9111", "the port of the service discover")
	flag.Parse()

	sd := &ServiceDiscover{
		RepoAddr: pipyRepoAddr,
		Client:   &http.Client{},
	}

	sd.Repos = make(map[string][]string)

	sdHandler := func(w http.ResponseWriter, r *http.Request) {
		sd.Update()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sd.GetPromTargets())
	}

	http.HandleFunc("/sd", sdHandler)

	log.Fatal().Err(http.ListenAndServe(fmt.Sprintf(":%s", sdPort), nil))
}
