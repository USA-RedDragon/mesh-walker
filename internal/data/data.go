package data

type Host struct {
	Name string `json:"name"`
}

type Interface struct {
	MAC  string `json:"mac"`
	Name string `json:"name"`
	IP   string `json:"ip,omitempty"`
}

type MeshRF struct {
	SSID             string `json:"ssid"`
	Channel          int    `json:"channel,string"`
	Status           string `json:"status"`
	Frequency        string `json:"freq"`
	ChannelBandwidth string `json:"chanbw"`
}

type NodeDetails struct {
	Description          string     `json:"description"`
	Model                string     `json:"model"`
	MeshGateway          BoolString `json:"mesh_gateway"`
	MeshSupernode        bool       `json:"mesh_supernode"`
	BoardID              string     `json:"board_id"`
	FirmwareManufacturer string     `json:"firmware_mfg"`
	FirmwareVersion      string     `json:"firmware_version"`
}

type LinkType string

const (
	LinkTypeTunnel    LinkType = "TUN"
	LinkTypeWireguard LinkType = "WIREGUARD"
	LinkTypeSupernode LinkType = "SUPER"
)

type LinkStatus string

type LinkInfo struct {
	HelloTime           int        `json:"helloTime"`
	LostLinkTime        int        `json:"lostLinkTime"`
	LinkQuality         float64    `json:"linkQuality"`
	Vtime               int        `json:"vtime"`
	LinkCost            float64    `json:"linkCost"`
	LinkType            LinkType   `json:"linkType"`
	Hostname            string     `json:"hostname"`
	PreviousLinkStatus  LinkStatus `json:"previousLinkStatus"`
	CurrentLinkStatus   LinkStatus `json:"currentLinkStatus"`
	NeighborLinkQuality float64    `json:"neighborLinkQuality"`
	SymmetryTime        float64    `json:"symmetryTime"`
	SequenceNumberValid bool       `json:"seqnoValid"`
	Pending             bool       `json:"pending"`
	LossHelloInterval   int        `json:"lossHelloInterval"`
	LossMultiplier      int        `json:"lossMultiplier"`
	Hysteresis          int        `json:"hysteresis"`
	SequenceNumber      int        `json:"seqno"`
	LossTime            int        `json:"lossTime"`
	ValidityTime        int        `json:"validityTime"`
	OLSRInterface       string     `json:"olsrInterface"`
	LastHelloTime       int        `json:"lastHelloTime"`
	AsymmetryTime       float64    `json:"asymmetryTime"`
}

type Response struct {
	Node             string                 `json:"node"`
	LastSeen         string                 `json:"lastseen"`
	Lat              string                 `json:"lat"`
	Lon              string                 `json:"lon"`
	MeshRF           MeshRF                 `json:"meshrf"`
	ChannelBandwidth int                    `json:"chanbw,string"`
	NodeDetails      NodeDetails            `json:"node_details"`
	Interfaces       []Interface            `json:"interfaces"`
	LinkInfo         map[string]LinkInfo    `json:"link_info"`
	LQM              map[string]interface{} `json:"lqm"`
	Hosts            []Host                 `json:"hosts"`
}
