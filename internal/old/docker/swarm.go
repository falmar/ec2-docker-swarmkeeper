package docker

type SwarmService interface {
	Inspect()
	Init(string, string) error
	Join(string, string, string) error
	Leave() error
}

type InfoInput struct {
}

type JoinSwarmInput struct {
	Addr []string `json:"addr"`
}

type JoinSwarmOutput struct {
}
