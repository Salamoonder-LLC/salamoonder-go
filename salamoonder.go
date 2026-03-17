package salamoonder

const (
	Version = "1.0.0"
	Author  = "Salamoonder"
	Email   = "support@salamoonder.com"
	License = "MIT"
)

type Salamoonder struct {
	client    *SalamoonderSession
	Task      *Tasks
	Akamai    *AkamaiWeb
	AkamaiSBSD *AkamaiSBSD
	Datadome  *Datadome
	Kasada    *Kasada
}

func New(apiKey string) (*Salamoonder, error) {
	client, err := NewSalamoonderSession(apiKey, "", "")
	if err != nil {
		return nil, err
	}

	return &Salamoonder{
		client:     client,
		Task:       NewTasks(client),
		Akamai:     NewAkamaiWeb(client),
		AkamaiSBSD: NewAkamaiSBSD(client),
		Datadome:   NewDatadome(client),
		Kasada:     NewKasada(client),
	}, nil
}

func NewWithOptions(apiKey, baseURL, impersonate string) (*Salamoonder, error) {
	client, err := NewSalamoonderSession(apiKey, baseURL, impersonate)
	if err != nil {
		return nil, err
	}

	return &Salamoonder{
		client:     client,
		Task:       NewTasks(client),
		Akamai:     NewAkamaiWeb(client),
		AkamaiSBSD: NewAkamaiSBSD(client),
		Datadome:   NewDatadome(client),
		Kasada:     NewKasada(client),
	}, nil
}

func (s *Salamoonder) Get(url string, opts *RequestOptions) (*Response, error) {
	return s.client.Get(url, opts)
}

func (s *Salamoonder) Post(url string, opts *RequestOptions) (*Response, error) {
	return s.client.Post(url, opts)
}

func (s *Salamoonder) Session() *SalamoonderSession {
	return s.client
}
