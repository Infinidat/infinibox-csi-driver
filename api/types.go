package api

type EndpointConfig struct {
	Endpoint string
	Version  string
	Username string
	Password string
}

type Volume struct {
	CgId                  int    `json:"cg_id,omitempty"`
	RmrTarget             bool   `json:"rmr_target,omitempty"`
	UpdatedAt             int    `json:"updated_at,omitempty"`
	NumBlocks             int    `json:"num_blocks,omitempty"`
	Allocated             int    `json:"allocated,omitempty"`
	Serial                string `json:"serial,omitempty"`
	Size                  int64  `json:"size,omitempty"`
	SsdEnabled            bool   `json:"ssd_enabled,omitempty"`
	ID                    int    `json:"id,omitempty"`
	ParentId              int    `json:"parent_id,omitempty"`
	CompressionSuppressed bool   `json:"compression_suppressed,omitempty"`
	Type                  string `json:"type,omitempty"`
	RmrSource             bool   `json:"rmr_source,omitempty"`
	Used                  int    `json:"used,omitempty"`
	TreeAllocated         int    `json:"tree_allocated,omitempty"`
	HasChildren           bool   `json:"has_children,omitempty"`
	DatasetType           string `json:"dataset_type,omitempty"`
	Provtype              string `json:"provtype,omitempty"`
	RmrSnapshotGuid       string `json:"rmr_snapshot_guid,omitempty"`
	CapacitySavings       int    `json:"capacity_savings,omitempty"`
	Name                  string `json:"name,omitempty"`
	CreatedAt             int64  `json:"created_at,omitempty"`
	PoolId                int64  `json:"pool_id,omitempty"`
	PoolName              string `json:"pool_name,omitempty"`
	CompressionEnabled    bool   `json:"compression_enabled,omitempty"`
	FamilyId              int    `json:"family_id,omitempty"`
	Depth                 int    `json:"depth,omitempty"`
	WriteProtected        bool   `json:"write_protected,omitempty"`
	Mapped                bool   `json:"mapped,omitempty"`
}

type VolumeParam struct {
	PoolId        int64  `json:"pool_id,omitempty"`
	VolumeSize    int64  `json:"size,omitempty"`
	Name          string `json:"name,omitempty"`
	ProvisionType string `json:"provtype,omitempty"`
	SsdEnabled    bool   `json:"ssd_enabled,omitempty"`
}

type VolumeResp struct {
	ID int `json:"id"`
}

type VolumeQeryIdByKeyParam struct {
	Name string `json:"name"`
}

type VolumeQeryBySelectedIdsParam struct {
	IDs []string `json:"ids"`
}

type StoragePool struct {
	ID                       int64    `json:"id"`
	Name                     string   `json:"name"`
	CreatedAt                int      `json:"created_at"`
	UpdatedAt                int      `json:"updated_at"`
	PhysicalCapacity         int      `json:"physical_capacity"`
	VirtualCapacity          int      `json:"virtual_capacity"`
	PhysicalCapacityWarning  int      `json:"physical_capacity_warning"`
	PhysicalCapacityCritical int      `json:"physical_capacity_critical"`
	State                    string   `json:"state"`
	ReservedCapacity         int      `json:"reserved_capacity"`
	MaxExtend                int      `json:"max_extend"`
	SsdEnabled               bool     `json:"ssd_enabled"`
	CompressionEnabled       bool     `json:"compression_enabled"`
	CapacitySavings          int      `json:"capacity_savings"`
	VolumesCount             int      `json:"volumes_count"`
	FilesystemsCount         int      `json:"filesystems_count"`
	SnapshotsCount           int      `json:"snapshots_count"`
	FilesystemSnapshotsCount int      `json:"filesystem_snapshots_count"`
	AllocatedPhysicalSpace   int      `json:"allocated_physical_space"`
	FreeVirtualSpace         int      `json:"free_virtual_space"`
	Owners                   []string `json:"owners"`
	QosPolicues              []string `json:"qos_policies"`
	EntitiesCount            int      `json:"entities_count"`
	FreePhysicalSpace        int      `json:"free_physical_space"`
}

type SnapshotVolumesResp struct {
	SnapShotID int    `json:"id,omitempty"`
	Size       int64  `json:"size,omitempty"`
	SsdEnabled bool   `json:"ssd_enabled,omitempty"`
	ParentID   int    `json:"parent_id,omitempty"`
	PoolID     int    `json:"pool_id,omitempty"`
	Name       string `json:"name,omitempty"`
}

type NetworkSpace struct {
	Properties          NetworkSpaceProperty `json:"properties,omitempty"`
	Service             string               `json:"service,omitempty"`
	Tenant_ID           int                  `json:"tenant_id,omitempty"`
	AutomaticIpFailback bool                 `json:"automatic_ip_failback,omitempty"`
	Interfaces          interface{}          `json:"interfaces,omitempty"`
	RateLimit           interface{}          `json:"rate_limit,omitempty"`
	ID                  int                  `json:"id,omitempty"`
	Portals             []Portal             `json:"ips,omitempty"`
	Mtu                 int                  `json:"mtu,omitempty"`
	NetworkConfig       NetworkConfigDetails `json:""network_config,omitempty"`
	Name                string               `json:"name,omitempty"`
	Vmac_Addresses      []VmacAddress        `json:"vmac_addresses,omitempty"`
	Routes              []Route              `json:"routes,omitempty"`
}
type Route struct {
	Netmask     int    `json:"netmask,omitempty"`
	Destination string `json:"destination,omitempty"`
	ID          int    `json:"id,omitempty"`
	Gateway     string `json:"gateway,omitempty"`
}
type VmacAddress struct {
	Role         string `json:"role,omitempty"`
	Vmac_Address string `json:"vmac_address,omitempty"`
}

type Portal struct {
	Type        string `json:"type,omitempty"`
	Tpgt        int    `json:"tpgt,omitempty"`
	IpAdress    string `json:"ip_address,omitempty"`
	VlanID      int    `json:"vlan_id,omitempty"`
	Enabled     bool   `json:"enabled,omitempty"`
	Reserved    bool   `json:"reserved,omitempty"`
	InterfaceID int    `json:"interface_id,omitempty"`
}

type NetworkConfigDetails struct {
	Netmask        int    `json:"netmask,omitempty"`
	Metwork        string `json:"network,omitempty"`
	DefaultGateway string `json:"default_gateway,omitempty"`
}

type NetworkSpaceProperty struct {
	IscsiServer         interface{} `json:"iscsi_isns_servers,omitempty"`
	IscsiIqn            string      `json:"iscsi_iqn,omitempty"`
	IscsiTcpPort        int         `json:"iscsi_tcp_port,omitempty"`
	IscsiSecurityMethod string      `json:"iscsi_default_security_method,omitempty"`
}

type HostCluster struct {
	ID    int    `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Hosts []Host `json:"hosts,omitempty"`
}

type Host struct {
	ID            int        `json:"id,omitempty"`
	Name          string     `json:"name,omitempty"`
	HostClusterID int        `json:"host_cluster_id,omitempty"`
	Ports         []HostPort `json:"ports,omitempty"`
	Luns          []LunInfo  `json:"luns,omitempty"`
}

type HostPort struct {
	HostID      int    `json:"host_id,omitempty"`
	PortType    string `json:"type,omitempty"`
	PortAddress string `json:"address,omitempty"`
}
type LunInfo struct {
	HostClusterID int  `json:"host_cluster_id,omitempty"`
	VolumeID      int  `json:"volume_id,omitempty"`
	CLustered     bool `json:"clustered,omitempty"`
	HostID        int  `json:"host_id,omitempty"`
	ID            int  `json:"id,omitempty"`
	Lun           int  `json:"lun,omitempty"`
}
type ExportFileSys struct {
	FilesystemID        int64                    `json:"filesystem_id,omitempty"`
	Name                string                   `json:"name,omitempty"`
	Transport_protocols string                   `json:"transport_protocols,omitempty"`
	Privileged_port     bool                     `json:"privileged_port,omitempty"`
	Export_path         string                   `json:"export_path,omitempty"`
	Permissionsput      []map[string]interface{} `json:"permissions,omitempty"`
}

type ExportResponse struct {
	InnerPath             string        `json:"inner_path,omitempty"`
	PrefWrite             int           `json:"pref_write,omitempty"`
	BitFileID             bool          `json:"32bit_file_id,omitempty"`
	PrefRead              int           `json:"pref_read,omitempty"`
	MaxRead               int           `json:"max_read,omitempty"`
	Permissions           []Permissions `json:"permissions,omitempty"`
	TenantId              int           `json:"tenant_id,omitempty"`
	CreatedAt             int           `json:"created_at,omitempty"`
	PrefReaddir           int           `json:"pref_readdir,omitempty"`
	Enabled               bool          `json:"enabled,omitempty"`
	UpdatedAt             int           `json:"updated_at,omitempty"`
	MakeAllUsersAnonymous bool          `json:"make_all_users_anonymous,omitempty"`
	SnapdirVisible        bool          `json:"snapdir_visible,omitempty"`
	TransportProtocols    string        `json:"transport_protocols,omitempty"`
	AnonymousGid          int           `json:"anonymous_gid,omitempty"`
	AnonymousUid          int           `json:"anonymous_uid,omitempty"`
	FilesystemId          int           `json:"filesystem_id,omitempty"`
	MaxWrite              int           `json:"max_write,omitempty"`
	PrivilegedPort        bool          `json:"privileged_port,omitempty"`
	ID                    int64         `json:"id,omitempty"`
	ExportPath            string        `json:"export_path,omitempty"`
}

type Permissions struct {
	Access       string `json:"access,omitempty"`
	NoRootSquash bool   `json:"no_root_squash,omitempty"`
	Client       string `json:"client,omitempty"`
}

type FilesystemRef struct {
	AtimeMode string `json:"atime_mode,omitempty"`
	PoolId    int    `json:"pool_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Provtype  string `json:"provtype,omitempty"`
	Size      int    `json:"size,omitempty"`
}

type FileSystem struct {
	ID         int64  `json:"id,omitempty"`
	PoolID     int64  `json:"pool_id,omitempty"`
	Name       string `json:"name,omitempty"`
	SsdEnabled bool   `json:"ssd_enabled,omitempty"`
	Provtype   string `json:"provtype,omitempty"`
	Size       int64  `json:"size,omitempty"`
	ParentID   int64  `json:"parent_id,omitempty"`
	PoolName   string `json:"pool_name,omitempty"`
	CreatedAt  int    `json:"created_at,omitempty"`
}

//FileSystemMetaData
type FileSystemMetaData struct {
	Ready           bool `json:"ready,omitempty"`
	NumberOfObjects int  `json:"number_of_objects,omitempty"`
	PageSize        int  `json:"page_size,omitempty"`
	PagesTotal      int  `json:"pages_total,omitempty"`
	Page            int  `json:"provtype,omitempty"`
}

type ExportPathRef struct {
	InnerPath          string        `json:"inner_path,omitempty"`
	PrefWrite          string        `json:"pref_write,omitempty"`
	PrefRead           int           `json:"pref_read,omitempty"`
	MaxRead            int           `json:"max_read,omitempty"`
	PrefReaddir        int           `json:"pref_readdir,omitempty"`
	TransportProtocols string        `json:"transport_protocols,omitempty"`
	FilesystemId       int           `json:"filesystem_id,omitempty"`
	MaxWrite           int           `json:"max_write,omitempty"`
	PrivilegedPort     bool          `json:"privileged_port,omitempty"`
	ExportPath         string        `json:"export_path,omitempty"`
	Permissions        []Permissions `json:"permissions,omitempty"`
}

type Metadata struct {
	ID         int    `json:"id,omitempty"`
	ObjectId   int    `json:"object_id,omitempty"`
	Key        string `json:"key,omitempty"`
	Value      string `json:"value,omitempty"`
	ObjectType string `json:"object_type,omitempty"`
}

//FileSystemSnapshot file system snapshot request parameter
type FileSystemSnapshot struct {
	ParentID       int64  `json:"parent_id"`
	SnapshotName   string `json:"name"`
	WriteProtected bool   `json:"write_protected"`
}

//FileSystemSnapshotResponce file system snapshot Response
type FileSystemSnapshotResponce struct {
	SnapshotID  int64  `json:"id"`
	Name        string `json:"name,omitempty"`
	DatasetType string `json:"dataset_type,omitempty"`
	ParentId    int64  `json:"parent_id,omitempty"`
	Size        int64  `json:"size,omitempty"`
	CreatedAt   int64  `json:"created_at,omitempty"`
}

type VolumeProtocolConfig struct {
	VolumeID      string
	StorageType   string
	ChildVolumeID string
}

//VolumeSnapshot volume snapshot request parameter
type VolumeSnapshot struct {
	ParentID       int    `json:"parent_id"`
	SnapshotName   string `json:"name"`
	WriteProtected bool   `json:"write_protected"`
	SsdEnabled     bool   `json:"ssd_enabled,omitempty"`
}
