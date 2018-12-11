package models

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"
	"yunion.io/x/pkg/tristate"
	"yunion.io/x/pkg/util/compare"
	"yunion.io/x/pkg/util/fileutils"
	"yunion.io/x/pkg/util/netutils"
	"yunion.io/x/pkg/util/osprofile"
	"yunion.io/x/pkg/util/regutils"
	"yunion.io/x/pkg/util/secrules"
	"yunion.io/x/pkg/util/timeutils"
	"yunion.io/x/pkg/utils"
	"yunion.io/x/sqlchemy"

	"yunion.io/x/onecloud/pkg/cloudcommon/consts"
	"yunion.io/x/onecloud/pkg/cloudcommon/db"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/lockman"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/quotas"
	"yunion.io/x/onecloud/pkg/cloudprovider"
	"yunion.io/x/onecloud/pkg/compute/options"
	"yunion.io/x/onecloud/pkg/compute/sshkeys"
	"yunion.io/x/onecloud/pkg/httperrors"
	"yunion.io/x/onecloud/pkg/mcclient"
	"yunion.io/x/onecloud/pkg/mcclient/auth"
	"yunion.io/x/onecloud/pkg/util/billing"
	"yunion.io/x/onecloud/pkg/util/seclib2"
)

const (
	VM_INIT            = "init"
	VM_UNKNOWN         = "unknown"
	VM_SCHEDULE        = "schedule"
	VM_SCHEDULE_FAILED = "sched_fail"
	VM_CREATE_NETWORK  = "network"
	VM_NETWORK_FAILED  = "net_fail"
	VM_DEVICE_FAILED   = "dev_fail"
	VM_CREATE_FAILED   = "create_fail"
	VM_CREATE_DISK     = "disk"
	VM_DISK_FAILED     = "disk_fail"
	VM_START_DEPLOY    = "start_deploy"
	VM_DEPLOYING       = "deploying"
	VM_DEPLOY_FAILED   = "deploy_fail"
	VM_READY           = "ready"
	VM_START_START     = "start_start"
	VM_STARTING        = "starting"
	VM_START_FAILED    = "start_fail" // # = ready
	VM_RUNNING         = "running"
	VM_START_STOP      = "start_stop"
	VM_STOPPING        = "stopping"
	VM_STOP_FAILED     = "stop_fail" // # = running

	VM_BACKUP_STARTING         = "backup_starting"
	VM_BACKUP_CREATING         = "backup_creating"
	VM_BACKUP_CREATE_FAILED    = "backup_create_fail"
	VM_DEPLOYING_BACKUP        = "deploying_backup"
	VM_DEPLOYING_BACKUP_FAILED = "deploging_backup_fail"

	VM_ATTACH_DISK_FAILED = "attach_disk_fail"
	VM_DETACH_DISK_FAILED = "detach_disk_fail"

	VM_START_SUSPEND  = "start_suspend"
	VM_SUSPENDING     = "suspending"
	VM_SUSPEND        = "suspend"
	VM_SUSPEND_FAILED = "suspend_failed"

	VM_START_DELETE = "start_delete"
	VM_DELETE_FAIL  = "delete_fail"
	VM_DELETING     = "deleting"

	VM_DEALLOCATED = "deallocated"

	VM_START_MIGRATE  = "start_migrate"
	VM_MIGRATING      = "migrating"
	VM_MIGRATE_FAILED = "migrate_failed"

	VM_CHANGE_FLAVOR      = "change_flavor"
	VM_CHANGE_FLAVOR_FAIL = "change_flavor_fail"
	VM_REBUILD_ROOT       = "rebuild_root"
	VM_REBUILD_ROOT_FAIL  = "rebuild_root_fail"

	VM_START_SNAPSHOT  = "snapshot_start"
	VM_SNAPSHOT        = "snapshot"
	VM_SNAPSHOT_DELETE = "snapshot_delete"
	VM_BLOCK_STREAM    = "block_stream"
	VM_MIRROR_FAIL     = "mirror_failed"
	VM_SNAPSHOT_SUCC   = "snapshot_succ"
	VM_SNAPSHOT_FAILED = "snapshot_failed"

	VM_SYNCING_STATUS = "syncing"
	VM_SYNC_CONFIG    = "sync_config"
	VM_SYNC_FAIL      = "sync_fail"

	VM_RESIZE_DISK      = "resize_disk"
	VM_START_SAVE_DISK  = "start_save_disk"
	VM_SAVE_DISK        = "save_disk"
	VM_SAVE_DISK_FAILED = "save_disk_failed"

	VM_RESTORING_SNAPSHOT = "restoring_snapshot"
	VM_RESTORE_DISK       = "restore_disk"
	VM_RESTORE_STATE      = "restore_state"
	VM_RESTORE_FAILED     = "restore_failed"

	VM_ASSOCIATE_EIP         = "associate_eip"
	VM_ASSOCIATE_EIP_FAILED  = "associate_eip_failed"
	VM_DISSOCIATE_EIP        = "dissociate_eip"
	VM_DISSOCIATE_EIP_FAILED = "dissociate_eip_failed"

	VM_REMOVE_STATEFILE = "remove_state"

	VM_ADMIN = "admin"

	SHUTDOWN_STOP      = "stop"
	SHUTDOWN_TERMINATE = "terminate"

	HYPERVISOR_KVM       = "kvm"
	HYPERVISOR_CONTAINER = "container"
	HYPERVISOR_BAREMETAL = "baremetal"
	HYPERVISOR_ESXI      = "esxi"
	HYPERVISOR_HYPERV    = "hyperv"
	HYPERVISOR_XEN       = "xen"

	HYPERVISOR_ALIYUN = "aliyun"
	HYPERVISOR_QCLOUD = "qcloud"
	HYPERVISOR_AZURE  = "azure"
	HYPERVISOR_AWS    = "aws"

	//	HYPERVISOR_DEFAULT = HYPERVISOR_KVM
	HYPERVISOR_DEFAULT = HYPERVISOR_KVM
)

var VM_RUNNING_STATUS = []string{VM_START_START, VM_STARTING, VM_RUNNING, VM_BLOCK_STREAM}
var VM_CREATING_STATUS = []string{VM_CREATE_NETWORK, VM_CREATE_DISK, VM_START_DEPLOY, VM_DEPLOYING}

var HYPERVISORS = []string{HYPERVISOR_KVM,
	HYPERVISOR_BAREMETAL,
	HYPERVISOR_ESXI,
	HYPERVISOR_CONTAINER,
	HYPERVISOR_ALIYUN,
	HYPERVISOR_AZURE,
	HYPERVISOR_AWS,
	HYPERVISOR_QCLOUD,
}

var PUBLIC_CLOUD_HYPERVISORS = []string{
	HYPERVISOR_ALIYUN,
	HYPERVISOR_AWS,
	HYPERVISOR_AZURE,
	HYPERVISOR_QCLOUD,
}

// var HYPERVISORS = []string{HYPERVISOR_ALIYUN}

var HYPERVISOR_HOSTTYPE = map[string]string{
	HYPERVISOR_KVM:       HOST_TYPE_HYPERVISOR,
	HYPERVISOR_BAREMETAL: HOST_TYPE_BAREMETAL,
	HYPERVISOR_ESXI:      HOST_TYPE_ESXI,
	HYPERVISOR_CONTAINER: HOST_TYPE_KUBELET,
	HYPERVISOR_ALIYUN:    HOST_TYPE_ALIYUN,
	HYPERVISOR_AZURE:     HOST_TYPE_AZURE,
	HYPERVISOR_AWS:       HOST_TYPE_AWS,
	HYPERVISOR_QCLOUD:    HOST_TYPE_QCLOUD,
}

var HOSTTYPE_HYPERVISOR = map[string]string{
	HOST_TYPE_HYPERVISOR: HYPERVISOR_KVM,
	HOST_TYPE_BAREMETAL:  HYPERVISOR_BAREMETAL,
	HOST_TYPE_ESXI:       HYPERVISOR_ESXI,
	HOST_TYPE_KUBELET:    HYPERVISOR_CONTAINER,
	HOST_TYPE_ALIYUN:     HYPERVISOR_ALIYUN,
	HOST_TYPE_AZURE:      HYPERVISOR_AZURE,
	HOST_TYPE_AWS:        HYPERVISOR_AWS,
	HOST_TYPE_QCLOUD:     HYPERVISOR_QCLOUD,
}

type SGuestManager struct {
	db.SVirtualResourceBaseManager
}

var GuestManager *SGuestManager

func init() {
	GuestManager = &SGuestManager{
		SVirtualResourceBaseManager: db.NewVirtualResourceBaseManager(
			SGuest{},
			"guests_tbl",
			"server",
			"servers",
		),
	}
	GuestManager.SetAlias("guest", "guests")
}

type SGuest struct {
	db.SVirtualResourceBase

	SBillingResourceBase

	VcpuCount int8 `nullable:"false" default:"1" list:"user" create:"optional"` // Column(TINYINT, nullable=False, default=1)
	VmemSize  int  `nullable:"false" list:"user" create:"required"`             // Column(Integer, nullable=False)

	BootOrder string `width:"8" charset:"ascii" nullable:"true" default:"cdn" list:"user" update:"user" create:"optional"` // Column(VARCHAR(8, charset='ascii'), nullable=True, default='cdn')

	DisableDelete    tristate.TriState `nullable:"false" default:"true" list:"user" update:"user" create:"optional"`           // Column(Boolean, nullable=False, default=True)
	ShutdownBehavior string            `width:"16" charset:"ascii" default:"stop" list:"user" update:"user" create:"optional"` // Column(VARCHAR(16, charset='ascii'), default=SHUTDOWN_STOP)

	KeypairId string `width:"36" charset:"ascii" nullable:"true" list:"user" create:"optional"` // Column(VARCHAR(36, charset='ascii'), nullable=True)

	HostId       string `width:"36" charset:"ascii" nullable:"true" list:"admin" get:"admin"` // Column(VARCHAR(36, charset='ascii'), nullable=True)
	BackupHostId string `width:"36" charset:"ascii" nullable:"true" list:"admin" get:"admin"`

	Vga     string `width:"36" charset:"ascii" nullable:"true" list:"user" update:"user" create:"optional"` // Column(VARCHAR(36, charset='ascii'), nullable=True)
	Vdi     string `width:"36" charset:"ascii" nullable:"true" list:"user" update:"user" create:"optional"` // Column(VARCHAR(36, charset='ascii'), nullable=True)
	Machine string `width:"36" charset:"ascii" nullable:"true" list:"user" update:"user" create:"optional"` // Column(VARCHAR(36, charset='ascii'), nullable=True)
	Bios    string `width:"36" charset:"ascii" nullable:"true" list:"user" update:"user" create:"optional"` // Column(VARCHAR(36, charset='ascii'), nullable=True)
	OsType  string `width:"36" charset:"ascii" nullable:"true" list:"user" update:"user" create:"optional"` // Column(VARCHAR(36, charset='ascii'), nullable=True)

	FlavorId string `width:"36" charset:"ascii" nullable:"true" list:"user" create:"optional"` // Column(VARCHAR(36, charset='ascii'), nullable=True)

	SecgrpId      string `width:"36" charset:"ascii" nullable:"true" get:"user" create:"optional"` // Column(VARCHAR(36, charset='ascii'), nullable=True)
	AdminSecgrpId string `width:"36" charset:"ascii" nullable:"true" get:"admin"`                  // Column(VARCHAR(36, charset='ascii'), nullable=True)

	Hypervisor string `width:"16" charset:"ascii" nullable:"false" default:"kvm" list:"user" create:"required"` // Column(VARCHAR(16, charset='ascii'), nullable=False, default=HYPERVISOR_DEFAULT)

	InstanceType string `width:"64" charset:"ascii" nullable:"true" list:"user" create:"optional"`
}

func (manager *SGuestManager) AllowListItems(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject) bool {
	if query.Contains("host") || query.Contains("wire") || query.Contains("zone") {
		if !db.IsAdminAllowList(userCred, manager) {
			return false
		}
	}
	return manager.SVirtualResourceBaseManager.AllowListItems(ctx, userCred, query)
}

func (manager *SGuestManager) ListItemFilter(ctx context.Context, q *sqlchemy.SQuery, userCred mcclient.TokenCredential, query jsonutils.JSONObject) (*sqlchemy.SQuery, error) {
	queryDict, ok := query.(*jsonutils.JSONDict)
	if !ok {
		return nil, fmt.Errorf("invalid querystring format")
	}

	billingTypeStr, _ := queryDict.GetString("billing_type")
	if len(billingTypeStr) > 0 {
		if billingTypeStr == BILLING_TYPE_POSTPAID {
			q = q.Filter(
				sqlchemy.OR(
					sqlchemy.IsNullOrEmpty(q.Field("billing_type")),
					sqlchemy.Equals(q.Field("billing_type"), billingTypeStr),
				),
			)
		} else {
			q = q.Equals("billing_type", billingTypeStr)
		}
		queryDict.Remove("billing_type")
	}

	q, err := manager.SVirtualResourceBaseManager.ListItemFilter(ctx, q, userCred, query)
	if err != nil {
		return nil, err
	}
	isBMstr, _ := queryDict.GetString("baremetal")
	if len(isBMstr) > 0 && utils.ToBool(isBMstr) {
		queryDict.Add(jsonutils.NewString(HYPERVISOR_BAREMETAL), "hypervisor")
		queryDict.Remove("baremetal")
	}
	hypervisor, _ := queryDict.GetString("hypervisor")
	if len(hypervisor) > 0 {
		q = q.Equals("hypervisor", hypervisor)
	}

	resourceTypeStr := jsonutils.GetAnyString(queryDict, []string{"resource_type"})
	if len(resourceTypeStr) > 0 {
		hosts := HostManager.Query().SubQuery()
		subq := hosts.Query(hosts.Field("id"))
		switch resourceTypeStr {
		case HostResourceTypeShared:
			subq = subq.Filter(
				sqlchemy.OR(
					sqlchemy.IsNullOrEmpty(hosts.Field("resource_type")),
					sqlchemy.Equals(hosts.Field("resource_type"), resourceTypeStr),
				),
			)
		default:
			subq = subq.Equals("resource_type", resourceTypeStr)
		}

		q = q.In("host_id", subq.SubQuery())
	}

	hostFilter, _ := queryDict.GetString("host")
	if len(hostFilter) > 0 {
		host, _ := HostManager.FetchByIdOrName(nil, hostFilter)
		if host == nil {
			return nil, httperrors.NewResourceNotFoundError("host %s not found", hostFilter)
		}
		if jsonutils.QueryBoolean(queryDict, "get_backup_guests_on_host", false) {
			q.Filter(sqlchemy.OR(sqlchemy.Equals(q.Field("host_id"), host.GetId()),
				sqlchemy.Equals(q.Field("backup_host_id"), host.GetId())))
		} else {
			q = q.Equals("host_id", host.GetId())
		}
	}

	secgrpFilter, _ := queryDict.GetString("secgroup")
	if len(secgrpFilter) > 0 {
		secgrp, _ := SecurityGroupManager.FetchByIdOrName(nil, secgrpFilter)
		if secgrp == nil {
			return nil, httperrors.NewResourceNotFoundError("secgroup %s not found", secgrpFilter)
		}
		q = q.Filter(
			sqlchemy.OR(
				sqlchemy.In(q.Field("id"), GuestsecgroupManager.Query("guest_id").Equals("secgroup_id", secgrp.GetId()).SubQuery()),
				sqlchemy.Equals(q.Field("secgrp_id"), secgrp.GetId()),
			),
		)
	}

	zoneFilter, _ := queryDict.GetString("zone")
	if len(zoneFilter) > 0 {
		zone, _ := ZoneManager.FetchByIdOrName(nil, zoneFilter)
		if zone == nil {
			return nil, httperrors.NewResourceNotFoundError("zone %s not found", zoneFilter)
		}
		hostTable := HostManager.Query().SubQuery()
		zoneTable := ZoneManager.Query().SubQuery()
		sq := hostTable.Query(hostTable.Field("id")).Join(zoneTable,
			sqlchemy.Equals(zoneTable.Field("id"), hostTable.Field("zone_id"))).Filter(sqlchemy.Equals(zoneTable.Field("id"), zone.GetId())).SubQuery()
		q = q.In("host_id", sq)
	}

	wireFilter, _ := queryDict.GetString("wire")
	if len(wireFilter) > 0 {
		wire, _ := WireManager.FetchByIdOrName(nil, wireFilter)
		if wire == nil {
			return nil, httperrors.NewResourceNotFoundError("wire %s not found", wireFilter)
		}
		hostTable := HostManager.Query().SubQuery()
		hostWire := HostwireManager.Query().SubQuery()
		sq := hostTable.Query(hostTable.Field("id")).Join(hostWire, sqlchemy.Equals(hostWire.Field("host_id"), hostTable.Field("id"))).Filter(sqlchemy.Equals(hostWire.Field("wire_id"), wire.GetId())).SubQuery()
		q = q.In("host_id", sq)
	}

	networkFilter, _ := queryDict.GetString("network")
	if len(networkFilter) > 0 {
		netI, _ := NetworkManager.FetchByIdOrName(userCred, networkFilter)
		if netI == nil {
			return nil, httperrors.NewResourceNotFoundError("network %s not found", networkFilter)
		}
		net := netI.(*SNetwork)
		hostTable := HostManager.Query().SubQuery()
		hostWire := HostwireManager.Query().SubQuery()
		sq := hostTable.Query(hostTable.Field("id")).Join(hostWire,
			sqlchemy.Equals(hostWire.Field("host_id"), hostTable.Field("id"))).Filter(sqlchemy.Equals(hostWire.Field("wire_id"), net.WireId)).SubQuery()
		q = q.In("host_id", sq)
	}

	diskFilter, _ := queryDict.GetString("disk")
	if len(diskFilter) > 0 {
		diskI, _ := DiskManager.FetchByIdOrName(userCred, diskFilter)
		if diskI == nil {
			return nil, httperrors.NewResourceNotFoundError("disk %s not found", diskFilter)
		}
		disk := diskI.(*SDisk)
		guestdisks := GuestdiskManager.Query().SubQuery()
		count := guestdisks.Query().Equals("disk_id", disk.Id).Count()
		if count > 0 {
			sgq := guestdisks.Query(guestdisks.Field("guest_id")).Equals("disk_id", disk.Id).SubQuery()
			q = q.Filter(sqlchemy.In(q.Field("id"), sgq))
		} else {
			hosts := HostManager.Query().SubQuery()
			hoststorages := HoststorageManager.Query().SubQuery()
			storages := StorageManager.Query().SubQuery()
			sq := hosts.Query(hosts.Field("id")).
				Join(hoststorages, sqlchemy.Equals(hoststorages.Field("host_id"), hosts.Field("id"))).
				Join(storages, sqlchemy.Equals(storages.Field("id"), hoststorages.Field("storage_id"))).
				Filter(sqlchemy.Equals(storages.Field("id"), disk.StorageId)).SubQuery()
			q = q.In("host_id", sq)
		}
	}

	managerFilter, _ := queryDict.GetString("manager")
	if len(managerFilter) > 0 {
		managerI, _ := CloudproviderManager.FetchByIdOrName(userCred, managerFilter)
		if managerI == nil {
			return nil, httperrors.NewResourceNotFoundError("cloud provider %s not found", managerFilter)
		}
		hosts := HostManager.Query().SubQuery()
		sq := hosts.Query(hosts.Field("id")).Equals("manager_id", managerI.GetId()).SubQuery()
		q = q.In("host_id", sq)
	}

	regionFilter, _ := queryDict.GetString("region")
	if len(regionFilter) > 0 {
		regionObj, err := CloudregionManager.FetchByIdOrName(userCred, regionFilter)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, httperrors.NewResourceNotFoundError("cloud region %s not found", regionFilter)
			} else {
				return nil, httperrors.NewGeneralError(err)
			}
		}
		hosts := HostManager.Query().SubQuery()
		zones := ZoneManager.Query().SubQuery()
		sq := hosts.Query(hosts.Field("id"))
		sq = sq.Join(zones, sqlchemy.Equals(hosts.Field("zone_id"), zones.Field("id")))
		sq = sq.Filter(sqlchemy.Equals(zones.Field("cloudregion_id"), regionObj.GetId()))
		q = q.In("host_id", sq)
	}

	withEip, _ := queryDict.GetString("with_eip")
	withoutEip, _ := queryDict.GetString("without_eip")
	if len(withEip) > 0 || len(withoutEip) > 0 {
		eips := ElasticipManager.Query().SubQuery()
		sq := eips.Query(eips.Field("associate_id")).Equals("associate_type", EIP_ASSOCIATE_TYPE_SERVER)
		sq = sq.IsNotNull("associate_id").IsNotEmpty("associate_id")

		if utils.ToBool(withEip) {
			q = q.In("id", sq)
		} else if utils.ToBool(withoutEip) {
			q = q.NotIn("id", sq)
		}
	}

	gpu, _ := queryDict.GetString("gpu")
	if len(gpu) != 0 {
		isodev := IsolatedDeviceManager.Query().SubQuery()
		sgq := isodev.Query(isodev.Field("guest_id")).
			Filter(sqlchemy.AND(
				sqlchemy.IsNotNull(isodev.Field("guest_id")),
				sqlchemy.Startswith(isodev.Field("dev_type"), "GPU")))
		showGpu := utils.ToBool(gpu)
		cond := sqlchemy.NotIn
		if showGpu {
			cond = sqlchemy.In
		}
		q = q.Filter(cond(q.Field("id"), sgq))
	}
	return q, nil
}

func (manager *SGuestManager) ExtraSearchConditions(ctx context.Context, q *sqlchemy.SQuery, like string) []sqlchemy.ICondition {
	var sq *sqlchemy.SSubQuery
	if regutils.MatchIP4Addr(like) {
		sq = GuestnetworkManager.Query("guest_id").Equals("ip_addr", like).SubQuery()
	} else if regutils.MatchMacAddr(like) {
		sq = GuestnetworkManager.Query("guest_id").Equals("mac_addr", like).SubQuery()
	}
	if sq != nil {
		return []sqlchemy.ICondition{sqlchemy.In(q.Field("id"), sq)}
	}
	return nil
}

func (guest *SGuest) GetHypervisor() string {
	if len(guest.Hypervisor) == 0 {
		return HYPERVISOR_DEFAULT
	} else {
		return guest.Hypervisor
	}
}

func (guest *SGuest) GetHostType() string {
	return HYPERVISOR_HOSTTYPE[guest.Hypervisor]
}

func (guest *SGuest) GetDriver() IGuestDriver {
	hypervisor := guest.GetHypervisor()
	if !utils.IsInStringArray(hypervisor, HYPERVISORS) {
		log.Fatalf("Unsupported hypervisor %s", hypervisor)
	}
	return GetDriver(hypervisor)
}

func (guest *SGuest) ValidateDeleteCondition(ctx context.Context) error {
	if guest.DisableDelete.IsTrue() {
		return httperrors.NewInvalidStatusError("Virtual server is locked, cannot delete")
	}
	if guest.IsValidPrePaid() {
		return httperrors.NewForbiddenError("not allow to delete prepaid server in valid status")
	}
	return guest.SVirtualResourceBase.ValidateDeleteCondition(ctx)
}

func (guest *SGuest) GetDisksQuery() *sqlchemy.SQuery {
	return GuestdiskManager.Query().Equals("guest_id", guest.Id)
}

func (guest *SGuest) DiskCount() int {
	return guest.GetDisksQuery().Count()
}

func (guest *SGuest) GetDisks() []SGuestdisk {
	disks := make([]SGuestdisk, 0)
	q := guest.GetDisksQuery().Asc("index")
	err := db.FetchModelObjects(GuestdiskManager, q, &disks)
	if err != nil {
		log.Errorf("Getdisks error: %s", err)
	}
	return disks
}

func (guest *SGuest) GetGuestDisk(diskId string) *SGuestdisk {
	guestdisk, err := db.NewModelObject(GuestdiskManager)
	if err != nil {
		log.Errorf("new guestdisk model failed: %s", err)
		return nil
	}
	q := guest.GetDisksQuery()
	err = q.Equals("disk_id", diskId).First(guestdisk)
	if err != nil {
		log.Errorf("GetGuestDisk error: %s", err)
		return nil
	}
	return guestdisk.(*SGuestdisk)
}

func (guest *SGuest) GetNetworksQuery() *sqlchemy.SQuery {
	return GuestnetworkManager.Query().Equals("guest_id", guest.Id)
}

func (guest *SGuest) NetworkCount() int {
	return guest.GetNetworksQuery().Count()
}

func (guest *SGuest) GetNetworks() []SGuestnetwork {
	guestnics := make([]SGuestnetwork, 0)
	q := guest.GetNetworksQuery().Asc("index")
	err := db.FetchModelObjects(GuestnetworkManager, q, &guestnics)
	if err != nil {
		log.Errorf("GetNetworks error: %s", err)
	}
	return guestnics
}

func (guest *SGuest) IsNetworkAllocated() bool {
	guestnics := guest.GetNetworks()
	for _, gn := range guestnics {
		if !gn.IsAllocated() {
			return false
		}
	}
	return true
}

func (guest *SGuest) CustomizeCreate(ctx context.Context, userCred mcclient.TokenCredential, ownerProjId string, query jsonutils.JSONObject, data jsonutils.JSONObject) error {
	guest.HostId = ""
	return guest.SVirtualResourceBase.CustomizeCreate(ctx, userCred, ownerProjId, query, data)
}

func (guest *SGuest) GetHost() *SHost {
	if len(guest.HostId) > 0 && regutils.MatchUUID(guest.HostId) {
		host, _ := HostManager.FetchById(guest.HostId)
		return host.(*SHost)
	}
	return nil
}

func (guest *SGuest) SetHostId(hostId string) error {
	_, err := guest.GetModelManager().TableSpec().Update(guest, func() error {
		guest.HostId = hostId
		return nil
	})
	return err
}

func (guest *SGuest) SetHostIdWithBackup(master, slave string) error {
	_, err := guest.GetModelManager().TableSpec().Update(guest, func() error {
		guest.HostId = master
		guest.BackupHostId = slave
		return nil
	})
	return err
}

func (guest *SGuest) ValidateResizeDisk(disk *SDisk, storage *SStorage) error {
	return guest.GetDriver().ValidateResizeDisk(guest, disk, storage)
}

func ValidateMemCpuData(data jsonutils.JSONObject) (int, int, error) {
	vmemSize := 0
	vcpuCount := 0
	var err error

	hypervisor, _ := data.GetString("hypervisor")
	if len(hypervisor) == 0 {
		hypervisor = HYPERVISOR_DEFAULT
	}
	driver := GetDriver(hypervisor)

	vmemStr, _ := data.GetString("vmem_size")
	if len(vmemStr) > 0 {
		if !regutils.MatchSize(vmemStr) {
			return 0, 0, httperrors.NewInputParameterError("Memory size must be number[+unit], like 256M, 1G or 256")
		}
		vmemSize, err = fileutils.GetSizeMb(vmemStr, 'M', 1024)
		if err != nil {
			return 0, 0, err
		}
		maxVmemGb := driver.GetMaxVMemSizeGB()
		if vmemSize < 8 || vmemSize > maxVmemGb*1024 {
			return 0, 0, httperrors.NewInputParameterError("Memory size must be 8MB ~ %d GB", maxVmemGb)
		}
	}
	vcpuStr, _ := data.GetString("vcpu_count")
	if len(vcpuStr) > 0 {
		if !regutils.MatchInteger(vcpuStr) {
			return 0, 0, httperrors.NewInputParameterError("CPU core count must be integer")
		}
		vcpuCount, _ = strconv.Atoi(vcpuStr)
		maxVcpuCount := driver.GetMaxVCpuCount()
		if vcpuCount < 1 || vcpuCount > maxVcpuCount {
			return 0, 0, httperrors.NewInputParameterError("CPU core count must be 1 ~ %d", maxVcpuCount)
		}
	}
	return vmemSize, vcpuCount, nil
}

func (self *SGuest) ValidateUpdateData(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data *jsonutils.JSONDict) (*jsonutils.JSONDict, error) {
	vmemSize, vcpuCount, err := ValidateMemCpuData(data)
	if err != nil {
		return nil, err
	}

	if vmemSize > 0 || vcpuCount > 0 {
		if !utils.IsInStringArray(self.Status, []string{VM_READY}) && self.GetHypervisor() != HYPERVISOR_CONTAINER {
			return nil, httperrors.NewInvalidStatusError("Cannot modify Memory and CPU in status %s", self.Status)
		}
		if self.GetHypervisor() == HYPERVISOR_BAREMETAL {
			return nil, httperrors.NewInputParameterError("Cannot modify memory for baremetal")
		}
	}

	if vmemSize > 0 {
		data.Add(jsonutils.NewInt(int64(vmemSize)), "vmem_size")
	}
	if vcpuCount > 0 {
		data.Add(jsonutils.NewInt(int64(vcpuCount)), "vcpu_count")
	}

	data, err = self.GetDriver().ValidateUpdateData(ctx, userCred, data)
	if err != nil {
		return nil, err
	}

	err = self.checkUpdateQuota(ctx, userCred, vcpuCount, vmemSize)
	if err != nil {
		return nil, httperrors.NewOutOfQuotaError(err.Error())
	}

	if data.Contains("name") {
		if name, _ := data.GetString("name"); len(name) < 2 {
			return nil, httperrors.NewInputParameterError("name is to short")
		}
	}
	return self.SVirtualResourceBase.ValidateUpdateData(ctx, userCred, query, data)
}

func (manager *SGuestManager) ValidateCreateData(ctx context.Context, userCred mcclient.TokenCredential, ownerProjId string, query jsonutils.JSONObject, data *jsonutils.JSONDict) (*jsonutils.JSONDict, error) {
	resetPassword := jsonutils.QueryBoolean(data, "reset_password", true)
	passwd, _ := data.GetString("password")
	if resetPassword && len(passwd) > 0 {
		if !seclib2.MeetComplxity(passwd) {
			return nil, httperrors.NewWeakPasswordError()
		}
	}

	var err error
	var hypervisor string
	var rootStorageType string
	var osProf osprofile.SOSProfile
	hypervisor, _ = data.GetString("hypervisor")
	if hypervisor != HYPERVISOR_CONTAINER {

		disk0Json, _ := data.Get("disk.0")
		if disk0Json == nil || disk0Json == jsonutils.JSONNull {
			return nil, httperrors.NewInputParameterError("No disk information provided")
		}
		diskConfig, err := parseDiskInfo(ctx, userCred, disk0Json)
		if err != nil {
			return nil, httperrors.NewInputParameterError("Invalid root image: %s", err)
		}
		if len(diskConfig.ImageId) > 0 && diskConfig.DiskType != DISK_TYPE_SYS {
			return nil, httperrors.NewBadRequestError("Snapshot error: disk index 0 but disk type is %s", diskConfig.DiskType)
		}

		if len(diskConfig.Backend) == 0 {
			diskConfig.Backend = STORAGE_LOCAL
		}
		rootStorageType = diskConfig.Backend

		data.Add(jsonutils.Marshal(diskConfig), "disk.0")

		imgProperties := diskConfig.ImageProperties
		if imgProperties == nil || len(imgProperties) == 0 {
			imgProperties = map[string]string{"os_type": "Linux"}
		}

		osType, _ := data.GetString("os_type")

		osProf, err = osprofile.GetOSProfileFromImageProperties(imgProperties, hypervisor)
		if err != nil {
			return nil, httperrors.NewInputParameterError("Invalid root image: %s", err)
		}

		if len(osProf.Hypervisor) > 0 && len(hypervisor) == 0 {
			hypervisor = osProf.Hypervisor
			data.Add(jsonutils.NewString(osProf.Hypervisor), "hypervisor")
		}
		if len(osProf.OSType) > 0 && len(osType) == 0 {
			osType = osProf.OSType
			data.Add(jsonutils.NewString(osProf.OSType), "os_type")
		}
		data.Add(jsonutils.Marshal(osProf), "__os_profile__")
	}

	data, err = ValidateScheduleCreateData(ctx, userCred, data, hypervisor)
	if err != nil {
		return nil, err
	}

	hypervisor, _ = data.GetString("hypervisor")
	if hypervisor != HYPERVISOR_CONTAINER {
		// support sku here
		var sku *SServerSku
		skuId := jsonutils.GetAnyString(data, []string{"sku", "flavor", "instance_type"})
		if len(skuId) > 0 {
			sku, err := ServerSkuManager.FetchSkuByNameAndHypervisor(skuId, hypervisor, true)
			if err != nil {
				return nil, err
			}

			data.Add(jsonutils.NewString(sku.Id), "instance_type")
			data.Add(jsonutils.NewInt(int64(sku.MemorySizeMB)), "vmem_size")
			data.Add(jsonutils.NewInt(int64(sku.CpuCoreCount)), "vcpu_count")
		} else {
			vmemSize, vcpuCount, err := ValidateMemCpuData(data)
			if err != nil {
				return nil, err
			}

			if vmemSize == 0 {
				return nil, httperrors.NewInputParameterError("Missing memory size")
			}
			if vcpuCount == 0 {
				vcpuCount = 1
			}
			data.Add(jsonutils.NewInt(int64(vmemSize)), "vmem_size")
			data.Add(jsonutils.NewInt(int64(vcpuCount)), "vcpu_count")
		}

		dataDiskDefs := make([]string, 0)
		if sku != nil && sku.AttachedDiskCount > 0 {
			for i := 0; i < sku.AttachedDiskCount; i += 1 {
				dataDiskDefs = append(dataDiskDefs, fmt.Sprintf("%dgb:%s", sku.AttachedDiskSizeGB, sku.AttachedDiskType))
			}
		}

		// start from data disk
		jsonArray := jsonutils.GetArrayOfPrefix(data, "disk")
		for idx := 1; idx < len(jsonArray); idx += 1 { // data.Contains(fmt.Sprintf("disk.%d", idx))
			diskJson, err := jsonArray[idx].GetString() // data.GetString(fmt.Sprintf("disk.%d", idx))
			if err != nil {
				return nil, httperrors.NewInputParameterError("invalid disk description %s", err)
			}
			dataDiskDefs = append(dataDiskDefs, diskJson)
		}

		for i := 0; i < len(dataDiskDefs); i += 1 {
			diskConfig, err := parseDiskInfo(ctx, userCred, jsonutils.NewString(dataDiskDefs[i]))
			if err != nil {
				return nil, httperrors.NewInputParameterError("parse disk description error %s", err)
			}
			if diskConfig.DiskType == DISK_TYPE_SYS {
				return nil, httperrors.NewBadRequestError("Snapshot error: disk index %d > 0 but disk type is %s", idx, DISK_TYPE_SYS)
			}
			if len(diskConfig.Backend) == 0 {
				diskConfig.Backend = rootStorageType
			}
			if len(diskConfig.Driver) == 0 {
				diskConfig.Driver = osProf.DiskDriver
			}
			data.Add(jsonutils.Marshal(diskConfig), fmt.Sprintf("disk.%d", i+1))
		}

		resourceTypeStr := jsonutils.GetAnyString(data, []string{"resource_type"})
		durationStr := jsonutils.GetAnyString(data, []string{"duration"})

		if len(durationStr) > 0 {

			if !userCred.IsAdminAllow(consts.GetServiceType(), manager.KeywordPlural(), "renew") {
				return nil, httperrors.NewForbiddenError("only admin can create prepaid resource")
			}

			if resourceTypeStr == HostResourceTypePrepaidRecycle {
				return nil, httperrors.NewConflictError("cannot create prepaid server on prepaid resource type")
			}

			billingCycle, err := billing.ParseBillingCycle(durationStr)
			if err != nil {
				return nil, httperrors.NewInputParameterError("invalid duration %s", durationStr)
			}

			if !GetDriver(hypervisor).IsSupportedBillingCycle(billingCycle) {
				return nil, httperrors.NewInputParameterError("unsupported duration %s", durationStr)
			}

			data.Add(jsonutils.NewString(BILLING_TYPE_PREPAID), "billing_type")
			data.Add(jsonutils.NewString(billingCycle.String()), "billing_cycle")
			// expired_at will be set later by callback
			// data.Add(jsonutils.NewTimeString(billingCycle.EndAt(time.Time{})), "expired_at")

			data.Set("duration", jsonutils.NewString(billingCycle.String()))
		}
	}

	netJsonArray := jsonutils.GetArrayOfPrefix(data, "net")
	for idx := 0; idx < len(netJsonArray); idx += 1 { // .Contains(fmt.Sprintf("net.%d", idx)); idx += 1 {
		netConfig, err := parseNetworkInfo(userCred, netJsonArray[idx])
		if err != nil {
			return nil, httperrors.NewInputParameterError("parse network description error %s", err)
		}
		err = isValidNetworkInfo(userCred, netConfig)
		if err != nil {
			return nil, err
		}
		if len(netConfig.Driver) == 0 {
			netConfig.Driver = osProf.NetDriver
		}
		data.Set(fmt.Sprintf("net.%d", idx), jsonutils.Marshal(netConfig))
	}

	isoDevArray := jsonutils.GetArrayOfPrefix(data, "isolated_device")
	for idx := 0; idx < len(isoDevArray); idx += 1 { // .Contains(fmt.Sprintf("isolated_device.%d", idx)); idx += 1 {
		if jsonutils.QueryBoolean(data, "backup", false) {
			return nil, httperrors.NewBadRequestError("Cannot create backup with isolated device")
		}
		devConfig, err := IsolatedDeviceManager.parseDeviceInfo(userCred, isoDevArray[idx])
		if err != nil {
			return nil, httperrors.NewInputParameterError("parse isolated device description error %s", err)
		}
		err = IsolatedDeviceManager.isValidDeviceinfo(devConfig)
		if err != nil {
			return nil, err
		}
		data.Set(fmt.Sprintf("isolated_device.%d", idx), jsonutils.Marshal(devConfig))
	}

	if data.Contains("cdrom") {
		cdromStr, err := data.GetString("cdrom")
		if err != nil {
			return nil, httperrors.NewInputParameterError("invalid cdrom device description %s", err)
		}
		cdromId, err := parseIsoInfo(ctx, userCred, cdromStr)
		if err != nil {
			return nil, httperrors.NewInputParameterError("parse cdrom device info error %s", err)
		}
		data.Add(jsonutils.NewString(cdromId), "cdrom")
	}

	keypairId, _ := data.GetString("keypair")
	if len(keypairId) == 0 {
		keypairId, _ = data.GetString("keypair_id")
	}
	if len(keypairId) > 0 {
		keypairObj, err := KeypairManager.FetchByIdOrName(userCred, keypairId)
		if err != nil {
			return nil, httperrors.NewResourceNotFoundError("Keypair %s not found", keypairId)
		}
		data.Add(jsonutils.NewString(keypairObj.GetId()), "keypair_id")
	} else {
		data.Add(jsonutils.NewString("None"), "keypair_id")
	}

	if data.Contains("secgroup") {
		secGrpId, _ := data.GetString("secgroup")
		secGrpObj, err := SecurityGroupManager.FetchByIdOrName(userCred, secGrpId)
		if err != nil {
			return nil, httperrors.NewResourceNotFoundError("Secgroup %s not found", secGrpId)
		}
		data.Add(jsonutils.NewString(secGrpObj.GetId()), "secgrp_id")
	} else {
		data.Add(jsonutils.NewString("default"), "secgrp_id")
	}

	/*
		TODO
		group
		for idx := 0; data.Contains(fmt.Sprintf("srvtag.%d", idx)); idx += 1 {

		}*/

	data, err = GetDriver(hypervisor).ValidateCreateData(ctx, userCred, data)
	if err != nil {
		return nil, err
	}

	data, err = manager.SVirtualResourceBaseManager.ValidateCreateData(ctx, userCred, ownerProjId, query, data)
	if err != nil {
		return nil, err
	}

	if !jsonutils.QueryBoolean(data, "is_system", false) {
		err = manager.checkCreateQuota(ctx, userCred, ownerProjId, data,
			jsonutils.QueryBoolean(data, "backup", false))
		if err != nil {
			return nil, err
		}
	}

	data.Add(jsonutils.NewString(ownerProjId), "owner_tenant_id")
	return data, nil
}

func (manager *SGuestManager) checkCreateQuota(ctx context.Context, userCred mcclient.TokenCredential, ownerProjId string, data *jsonutils.JSONDict, hasBackup bool) error {
	req := getGuestResourceRequirements(ctx, userCred, data, 1, hasBackup)
	err := QuotaManager.CheckSetPendingQuota(ctx, userCred, ownerProjId, &req)
	if err != nil {
		return httperrors.NewOutOfQuotaError(err.Error())
	} else {
		return nil
	}
}

func (self *SGuest) checkUpdateQuota(ctx context.Context, userCred mcclient.TokenCredential, vcpuCount int, vmemSize int) error {
	req := SQuota{}

	if vcpuCount > 0 && vcpuCount > int(self.VcpuCount) {
		req.Cpu = vcpuCount - int(self.VcpuCount)
	}

	if vmemSize > 0 && vmemSize > self.VmemSize {
		req.Memory = vmemSize - self.VmemSize
	}

	_, err := QuotaManager.CheckQuota(ctx, userCred, self.ProjectId, &req)

	return err
}

func getGuestResourceRequirements(ctx context.Context, userCred mcclient.TokenCredential, data jsonutils.JSONObject, count int, hasBackup bool) SQuota {
	vcpuCount, _ := data.Int("vcpu_count")
	if vcpuCount == 0 {
		vcpuCount = 1
	}

	vmemSize, _ := data.Int("vmem_size")

	diskSize := 0

	diskJsonArray := jsonutils.GetArrayOfPrefix(data, "disk")
	for idx := 0; idx < len(diskJsonArray); idx += 1 { // data.Contains(fmt.Sprintf("disk.%d", idx)); idx += 1 {
		diskConfig, _ := parseDiskInfo(ctx, userCred, diskJsonArray[idx])
		diskSize += diskConfig.SizeMb
	}

	isoDevArray := jsonutils.GetArrayOfPrefix(data, "isolated_device")
	devCount := len(isoDevArray)
	// for idx := 0; data.Contains(fmt.Sprintf("isolated_device.%d", idx)); idx += 1 {
	// 	devCount += 1
	//}

	eNicCnt := 0
	iNicCnt := 0
	eBw := 0
	iBw := 0
	netJsonArray := jsonutils.GetArrayOfPrefix(data, "net")
	for idx := 0; idx < len(netJsonArray); idx += 1 { // .Contains(fmt.Sprintf("net.%d", idx)); idx += 1 {
		netConfig, _ := parseNetworkInfo(userCred, netJsonArray[idx])
		if isExitNetworkInfo(netConfig) {
			eNicCnt += 1
			eBw += netConfig.BwLimit
		} else {
			iNicCnt += 1
			iBw += netConfig.BwLimit
		}
	}
	if hasBackup {
		vcpuCount = vcpuCount * 2
		vmemSize = vmemSize * 2
		diskSize = diskSize * 2
	}
	return SQuota{
		Cpu:            int(vcpuCount) * count,
		Memory:         int(vmemSize) * count,
		Storage:        diskSize * count,
		Port:           iNicCnt * count,
		Eport:          eNicCnt * count,
		Bw:             iBw * count,
		Ebw:            eBw * count,
		IsolatedDevice: devCount * count,
	}
}

func (guest *SGuest) getGuestBackupResourceRequirements(ctx context.Context, userCred mcclient.TokenCredential) SQuota {
	guestDisksSize := guest.getDiskSize()
	return SQuota{
		Cpu:     int(guest.VcpuCount),
		Memory:  guest.VmemSize,
		Storage: guestDisksSize,
	}
}

func (guest *SGuest) PostCreate(ctx context.Context, userCred mcclient.TokenCredential, ownerProjId string, query jsonutils.JSONObject, data jsonutils.JSONObject) {
	guest.SVirtualResourceBase.PostCreate(ctx, userCred, ownerProjId, query, data)
	tags := []string{"cpu_bound", "io_bound", "io_hardlimit"}
	appTags := make([]string, 0)
	for _, tag := range tags {
		if data.Contains(tag) {
			appTags = append(appTags, tag)
		}
	}
	guest.setApptags(ctx, appTags, userCred)
	osProfileJson, _ := data.Get("__os_profile__")
	if osProfileJson != nil {
		guest.setOSProfile(ctx, userCred, osProfileJson)
	}

	userData, _ := data.GetString("user_data")
	if len(userData) > 0 {
		guest.setUserData(ctx, userCred, userData)
	}
}

func (guest *SGuest) setApptags(ctx context.Context, appTags []string, userCred mcclient.TokenCredential) {
	err := guest.SetMetadata(ctx, "app_tags", strings.Join(appTags, ","), userCred)
	if err != nil {
		log.Errorln(err)
	}
}

func (manager *SGuestManager) OnCreateComplete(ctx context.Context, items []db.IModel, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) {
	pendingUsage := getGuestResourceRequirements(ctx, userCred, data, len(items),
		jsonutils.QueryBoolean(data, "backup", false))
	RunBatchCreateTask(ctx, items, userCred, data, pendingUsage, "GuestBatchCreateTask")
}

func (guest *SGuest) GetGroups() []SGroupguest {
	guestgroups := make([]SGroupguest, 0)
	q := GroupguestManager.Query().Equals("guest_id", guest.Id)
	err := db.FetchModelObjects(GroupguestManager, q, &guestgroups)
	if err != nil {
		log.Errorf("GetGroups fail %s", err)
		return nil
	}
	return guestgroups
}

func (self *SGuest) getBandwidth(isExit bool) int {
	bw := 0
	networks := self.GetNetworks()
	if networks != nil && len(networks) > 0 {
		for i := 0; i < len(networks); i += 1 {
			if networks[i].IsExit() == isExit {
				bw += networks[i].getBandwidth()
			}
		}
	}
	return bw
}

func (self *SGuest) getExtBandwidth() int {
	return self.getBandwidth(true)
}

func (self *SGuest) GetCustomizeColumns(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject) *jsonutils.JSONDict {
	extra := self.SVirtualResourceBase.GetCustomizeColumns(ctx, userCred, query)

	if db.IsAdminAllowGet(userCred, self) {
		host := self.GetHost()
		if host != nil {
			extra.Add(jsonutils.NewString(host.Name), "host")
		}
	}
	extra.Add(jsonutils.NewString(strings.Join(self.getRealIPs(), ",")), "ips")
	eip, _ := self.GetEip()
	if eip != nil {
		extra.Add(jsonutils.NewString(eip.IpAddr), "eip")
		extra.Add(jsonutils.NewString(eip.Mode), "eip_mode")
	}
	extra.Add(jsonutils.NewInt(int64(self.getDiskSize())), "disk")
	// flavor??
	// extra.Add(jsonutils.NewString(self.getFlavorName()), "flavor")
	extra.Add(jsonutils.NewString(self.getKeypairName()), "keypair")
	extra.Add(jsonutils.NewInt(int64(self.getExtBandwidth())), "ext_bw")

	extra.Add(jsonutils.NewString(self.GetSecgroupName()), "secgroup")

	if secgroups := self.getSecgroupJson(); len(secgroups) > 0 {
		extra.Add(jsonutils.NewArray(secgroups...), "secgroups")
	}

	if self.PendingDeleted {
		pendingDeletedAt := self.PendingDeletedAt.Add(time.Second * time.Duration(options.Options.PendingDeleteExpireSeconds))
		extra.Add(jsonutils.NewString(timeutils.FullIsoTime(pendingDeletedAt)), "auto_delete_at")
	}

	isGpu := jsonutils.JSONFalse
	if self.isGpu() {
		isGpu = jsonutils.JSONTrue
	}
	extra.Add(isGpu, "is_gpu")

	extra.Add(jsonutils.JSONNull, "cdrom")
	if cdrom := self.getCdrom(); cdrom != nil {
		extra.Set("cdrom", jsonutils.NewString(cdrom.GetDetails()))
	}

	return self.moreExtraInfo(extra)
}

func (self *SGuest) moreExtraInfo(extra *jsonutils.JSONDict) *jsonutils.JSONDict {
	zone := self.getZone()
	if zone != nil {
		extra.Add(jsonutils.NewString(zone.GetId()), "zone_id")
		extra.Add(jsonutils.NewString(zone.GetName()), "zone")
		if len(zone.ExternalId) > 0 {
			extra.Add(jsonutils.NewString(zone.ExternalId), "zone_external_id")
		}

		region := zone.GetRegion()
		if region != nil {
			extra.Add(jsonutils.NewString(region.Id), "region_id")
			extra.Add(jsonutils.NewString(region.Name), "region")

			if len(region.ExternalId) > 0 {
				extra.Add(jsonutils.NewString(region.ExternalId), "region_external_id")
			}
		}

		host := self.GetHost()
		if host != nil {
			provider := host.GetCloudprovider()
			if provider != nil {
				extra.Add(jsonutils.NewString(host.ManagerId), "manager_id")
				extra.Add(jsonutils.NewString(provider.GetName()), "manager")
			}
		}
	}

	err := self.CanPerformPrepaidRecycle()
	if err != nil {
		extra.Add(jsonutils.JSONFalse, "can_recycle")
	} else {
		extra.Add(jsonutils.JSONTrue, "can_recycle")
	}

	return extra
}

func (self *SGuest) GetExtraDetails(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject) *jsonutils.JSONDict {
	extra := self.SVirtualResourceBase.GetExtraDetails(ctx, userCred, query)
	extra.Add(jsonutils.NewString(self.getNetworksDetails()), "networks")
	extra.Add(jsonutils.NewString(self.getDisksDetails()), "disks")
	extra.Add(self.getDisksInfoDetails(), "disks_info")
	extra.Add(jsonutils.NewInt(int64(self.getDiskSize())), "disk")
	cdrom := self.getCdrom()
	if cdrom != nil {
		extra.Add(jsonutils.NewString(cdrom.GetDetails()), "cdrom")
	}
	// extra.Add(jsonutils.NewString(self.getFlavorName()), "flavor")
	extra.Add(jsonutils.NewString(self.getKeypairName()), "keypair")
	extra.Add(jsonutils.NewString(self.GetSecgroupName()), "secgroup")

	if secgroups := self.getSecgroupJson(); len(secgroups) > 0 {
		extra.Add(jsonutils.NewArray(secgroups...), "secgroups")
	}

	extra.Add(jsonutils.NewString(strings.Join(self.getIPs(), ",")), "ips")
	extra.Add(jsonutils.NewString(self.getSecurityRules()), "security_rules")
	extra.Add(jsonutils.NewString(self.getIsolatedDeviceDetails()), "isolated_devices")
	osName := self.GetOS()
	if len(osName) > 0 {
		extra.Add(jsonutils.NewString(osName), "os_name")
	}
	if metaData, err := self.GetAllMetadata(userCred); err == nil {
		extra.Add(jsonutils.Marshal(metaData), "metadata")
	}
	if db.IsAdminAllowGet(userCred, self) {
		host := self.GetHost()
		if host != nil {
			extra.Add(jsonutils.NewString(host.GetName()), "host")
		}
		extra.Add(jsonutils.NewString(self.getAdminSecurityRules()), "admin_security_rules")
	}
	eip, _ := self.GetEip()
	if eip != nil {
		extra.Add(jsonutils.NewString(eip.IpAddr), "eip")
		extra.Add(jsonutils.NewString(eip.Mode), "eip_mode")
	}

	isGpu := jsonutils.JSONFalse
	if self.isGpu() {
		isGpu = jsonutils.JSONTrue
	}
	extra.Add(isGpu, "is_gpu")

	if self.IsPrepaidRecycle() {
		extra.Add(jsonutils.JSONTrue, "is_prepaid_recycle")
	} else {
		extra.Add(jsonutils.JSONFalse, "is_prepaid_recycle")
	}

	return self.moreExtraInfo(extra)
}

func (manager *SGuestManager) ListItemExportKeys(ctx context.Context, q *sqlchemy.SQuery, userCred mcclient.TokenCredential, query jsonutils.JSONObject) (*sqlchemy.SQuery, error) {
	exportKeys, _ := query.GetString("export_keys")
	keys := strings.Split(exportKeys, ",")

	// guest_id as filter key
	if utils.IsInStringArray("ips", keys) {
		guestIpsQuery := GuestnetworkManager.Query("guest_id").GroupBy("guest_id")
		guestIpsQuery.AppendField(sqlchemy.GROUP_CONCAT("concat_ip_addr", guestIpsQuery.Field("ip_addr")))
		ipsSubQuery := guestIpsQuery.SubQuery()
		guestIpsQuery.DebugQuery()
		q.LeftJoin(ipsSubQuery, sqlchemy.Equals(q.Field("id"), ipsSubQuery.Field("guest_id")))
		q.AppendField(ipsSubQuery.Field("concat_ip_addr"))
	}
	if utils.IsInStringArray("disk", keys) {
		guestDisksQuery := GuestdiskManager.Query("guest_id", "disk_id").GroupBy("guest_id")
		diskQuery := DiskManager.Query("id", "disk_size").SubQuery()
		guestDisksQuery.Join(diskQuery, sqlchemy.Equals(diskQuery.Field("id"), guestDisksQuery.Field("disk_id")))
		guestDisksQuery.AppendField(sqlchemy.SUM("disk_size", diskQuery.Field("disk_size")))
		guestDisksSubQuery := guestDisksQuery.SubQuery()
		guestDisksSubQuery.DebugQuery()
		q.LeftJoin(guestDisksSubQuery, sqlchemy.Equals(q.Field("id"), guestDisksSubQuery.
			Field("guest_id")))
		q.AppendField(guestDisksSubQuery.Field("disk_size"))
	}
	if utils.IsInStringArray("eip", keys) {
		eipsQuery := ElasticipManager.Query("associate_id", "ip_addr").Equals("associate_type", "server").GroupBy("associate_id")
		eipsSubQuery := eipsQuery.SubQuery()
		eipsSubQuery.DebugQuery()
		q.LeftJoin(eipsSubQuery, sqlchemy.Equals(q.Field("id"), eipsSubQuery.Field("associate_id")))
		q.AppendField(eipsSubQuery.Field("ip_addr", "eip"))
	}

	// host_id as filter key
	if utils.IsInStringArray("region", keys) {
		zoneQuery := ZoneManager.Query("id", "cloudregion_id").SubQuery()
		hostQuery := HostManager.Query("id", "zone_id").GroupBy("id")
		cloudregionQuery := CloudregionManager.Query("id", "name").SubQuery()
		hostQuery.LeftJoin(zoneQuery, sqlchemy.Equals(hostQuery.Field("zone_id"), zoneQuery.Field("id"))).
			LeftJoin(cloudregionQuery, sqlchemy.OR(sqlchemy.Equals(cloudregionQuery.Field("id"),
				zoneQuery.Field("cloudregion_id")), sqlchemy.Equals(cloudregionQuery.Field("id"), "default")))
		hostQuery.AppendField(cloudregionQuery.Field("name", "region"))
		hostSubQuery := hostQuery.SubQuery()
		q.LeftJoin(hostSubQuery, sqlchemy.Equals(q.Field("host_id"), hostSubQuery.Field("id")))
		q.AppendField(hostSubQuery.Field("region"))
	}
	if utils.IsInStringArray("manager", keys) {
		hostQuery := HostManager.Query("id", "manager_id").GroupBy("id")
		cloudProviderQuery := CloudproviderManager.Query("id", "name").SubQuery()
		hostQuery.LeftJoin(cloudProviderQuery, sqlchemy.Equals(hostQuery.Field("manager_id"),
			cloudProviderQuery.Field("id")))
		hostQuery.AppendField(cloudProviderQuery.Field("name", "manager"))
		hostSubQuery := hostQuery.SubQuery()
		q.LeftJoin(hostSubQuery, sqlchemy.Equals(q.Field("host_id"), hostSubQuery.Field("id")))
		q.AppendField(hostSubQuery.Field("manager"))
	}
	return q, nil
}

func (manager *SGuestManager) GetExportExtraKeys(ctx context.Context, query jsonutils.JSONObject, rowMap map[string]string) *jsonutils.JSONDict {
	res := manager.SStatusStandaloneResourceBaseManager.GetExportExtraKeys(ctx, query, rowMap)
	exportKeys, _ := query.GetString("export_keys")
	keys := strings.Split(exportKeys, ",")
	if ips, ok := rowMap["concat_ip_addr"]; ok && len(ips) > 0 {
		res.Set("ips", jsonutils.NewString(ips))
	}
	if eip, ok := rowMap["eip"]; ok && len(eip) > 0 {
		res.Set("eip", jsonutils.NewString(eip))
	}
	if disk, ok := rowMap["disk_size"]; ok {
		res.Set("disk", jsonutils.NewString(disk))
	}
	if region, ok := rowMap["region"]; ok && len(region) > 0 {
		res.Set("region", jsonutils.NewString(region))
	}
	if manager, ok := rowMap["manager"]; ok && len(manager) > 0 {
		res.Set("manager", jsonutils.NewString(manager))
	}
	if utils.IsInStringArray("tenant", keys) {
		if projectId, ok := rowMap["tenant_id"]; ok {
			tenant, err := db.TenantCacheManager.FetchTenantById(ctx, projectId)
			if err == nil {
				res.Set("tenant", jsonutils.NewString(tenant.GetName()))
			}
		}
	}
	if utils.IsInStringArray("os_distribution", keys) {
		if osType, ok := rowMap["os_type"]; ok {
			res.Set("os_distribution", jsonutils.NewString(osType))
		}
	}
	return res
}

func (self *SGuest) getNetworksDetails() string {
	var buf bytes.Buffer
	for _, nic := range self.GetNetworks() {
		buf.WriteString(nic.GetDetailedString())
		buf.WriteString("\n")
	}
	return buf.String()
}

func (self *SGuest) getDisksDetails() string {
	var buf bytes.Buffer
	for _, disk := range self.GetDisks() {
		buf.WriteString(disk.GetDetailedString())
		buf.WriteString("\n")
	}
	return buf.String()
}

func (self *SGuest) getDisksInfoDetails() *jsonutils.JSONArray {
	details := jsonutils.NewArray()
	for _, disk := range self.GetDisks() {
		details.Add(disk.GetDetailedJson())
	}
	return details
}

func (self *SGuest) getIsolatedDeviceDetails() string {
	var buf bytes.Buffer
	for _, dev := range self.GetIsolatedDevices() {
		buf.WriteString(dev.getDetailedString())
		buf.WriteString("\n")
	}
	return buf.String()
}

func (self *SGuest) getDiskSize() int {
	size := 0
	for _, disk := range self.GetDisks() {
		size += disk.GetDisk().DiskSize
	}
	return size
}

func (self *SGuest) getCdrom() *SGuestcdrom {
	cdrom := SGuestcdrom{}
	cdrom.SetModelManager(GuestcdromManager)

	err := GuestcdromManager.Query().Equals("id", self.Id).First(&cdrom)
	if err != nil {
		if err == sql.ErrNoRows {
			cdrom.Id = self.Id
			err = GuestcdromManager.TableSpec().Insert(&cdrom)
			if err != nil {
				log.Errorf("insert cdrom fail %s", err)
				return nil
			}
			return &cdrom
		} else {
			log.Errorf("getCdrom query fail %s", err)
			return nil
		}
	} else {
		return &cdrom
	}
}

func (self *SGuest) getKeypair() *SKeypair {
	if len(self.KeypairId) > 0 {
		keypair, _ := KeypairManager.FetchById(self.KeypairId)
		if keypair != nil {
			return keypair.(*SKeypair)
		}
	}
	return nil
}

func (self *SGuest) getKeypairName() string {
	keypair := self.getKeypair()
	if keypair != nil {
		return keypair.Name
	}
	return ""
}

func (self *SGuest) getNotifyIps() []string {
	ips := self.getRealIPs()
	vips := self.getVirtualIPs()
	if vips != nil {
		ips = append(ips, vips...)
	}
	return ips
}

func (self *SGuest) getRealIPs() []string {
	ips := make([]string, 0)
	for _, nic := range self.GetNetworks() {
		if !nic.Virtual {
			ips = append(ips, nic.IpAddr)
		}
	}
	return ips
}

func (self *SGuest) IsExitOnly() bool {
	for _, ip := range self.getRealIPs() {
		addr, _ := netutils.NewIPV4Addr(ip)
		if !netutils.IsExitAddress(addr) {
			return false
		}
	}
	return true
}

func (self *SGuest) getVirtualIPs() []string {
	ips := make([]string, 0)
	for _, guestgroup := range self.GetGroups() {
		group := guestgroup.GetGroup()
		for _, groupnetwork := range group.GetNetworks() {
			ips = append(ips, groupnetwork.IpAddr)
		}
	}
	return ips
}

func (self *SGuest) getIPs() []string {
	ips := self.getRealIPs()
	vips := self.getVirtualIPs()
	ips = append(ips, vips...)
	/*eip, _ := self.GetEip()
	if eip != nil {
		ips = append(ips, eip.IpAddr)
	}*/
	return ips
}

func (self *SGuest) getZone() *SZone {
	host := self.GetHost()
	if host != nil {
		return host.GetZone()
	}
	return nil
}

func (self *SGuest) getRegion() *SCloudregion {
	zone := self.getZone()
	if zone != nil {
		return zone.GetRegion()
	}
	return nil
}

func (self *SGuest) GetOS() string {
	if len(self.OsType) > 0 {
		return self.OsType
	}
	return self.GetMetadata("os_name", nil)
}

func (self *SGuest) IsLinux() bool {
	os := self.GetOS()
	if strings.HasPrefix(strings.ToLower(os), "lin") {
		return true
	} else {
		return false
	}
}

func (self *SGuest) IsWindows() bool {
	os := self.GetOS()
	if strings.HasPrefix(strings.ToLower(os), "win") {
		return true
	} else {
		return false
	}
}

func (self *SGuest) getSecgroupJson() []jsonutils.JSONObject {
	secgroups := []jsonutils.JSONObject{}
	for _, secGrp := range self.GetSecgroups() {
		secgroups = append(secgroups, secGrp.getDesc())
	}
	return secgroups
}

func (self *SGuest) GetSecgroups() []SSecurityGroup {
	secgrpQuery := SecurityGroupManager.Query()
	secgrpQuery.Filter(
		sqlchemy.OR(
			sqlchemy.Equals(secgrpQuery.Field("id"), self.SecgrpId),
			sqlchemy.In(secgrpQuery.Field("id"), GuestsecgroupManager.Query("secgroup_id").Equals("guest_id", self.Id).SubQuery()),
		),
	)
	secgroups := []SSecurityGroup{}
	if err := db.FetchModelObjects(SecurityGroupManager, secgrpQuery, &secgroups); err != nil {
		log.Errorf("Get security group error: %v", err)
		return nil
	}
	return secgroups
}

func (self *SGuest) getSecgroup() *SSecurityGroup {
	return SecurityGroupManager.FetchSecgroupById(self.SecgrpId)
}

func (self *SGuest) getAdminSecgroup() *SSecurityGroup {
	return SecurityGroupManager.FetchSecgroupById(self.AdminSecgrpId)
}

func (self *SGuest) GetSecgroupName() string {
	secgrp := self.getSecgroup()
	if secgrp != nil {
		return secgrp.GetName()
	}
	return ""
}

func (self *SGuest) getAdminSecgroupName() string {
	secgrp := self.getAdminSecgroup()
	if secgrp != nil {
		return secgrp.GetName()
	}
	return ""
}

func (self *SGuest) GetSecRules() []secrules.SecurityRule {
	return self.getSecRules()
}

func (self *SGuest) getSecRules() []secrules.SecurityRule {
	if secgrp := self.getSecgroup(); secgrp != nil {
		return secgrp.GetSecRules("")
	}
	if rule, err := secrules.ParseSecurityRule(options.Options.DefaultSecurityRules); err == nil {
		return []secrules.SecurityRule{*rule}
	} else {
		log.Errorf("Default SecurityRules error: %v", err)
	}
	return []secrules.SecurityRule{}
}

func (self *SGuest) getSecurityRules() string {
	secgrp := self.getSecgroup()
	if secgrp != nil {
		return secgrp.getSecurityRuleString("")
	} else {
		return options.Options.DefaultSecurityRules
	}
}

//获取多个安全组规则，优先级降序排序
func (self *SGuest) getSecurityGroupsRules() string {
	secgroups := self.GetSecgroups()
	secgroupids := []string{}
	for _, secgroup := range secgroups {
		secgroupids = append(secgroupids, secgroup.Id)
	}
	q := SecurityGroupRuleManager.Query()
	q.Filter(sqlchemy.In(q.Field("secgroup_id"), secgroupids)).Desc(q.Field("priority"))
	secrules := []SSecurityGroupRule{}
	if err := db.FetchModelObjects(SecurityGroupRuleManager, q, &secrules); err != nil {
		log.Errorf("Get rules error: %v", err)
		return options.Options.DefaultSecurityRules
	}
	rules := []string{}
	for _, rule := range secrules {
		rules = append(rules, rule.String())
	}
	return strings.Join(rules, SECURITY_GROUP_SEPARATOR)
}

func (self *SGuest) getAdminSecurityRules() string {
	secgrp := self.getAdminSecgroup()
	if secgrp != nil {
		return secgrp.getSecurityRuleString("")
	} else {
		return options.Options.DefaultAdminSecurityRules
	}
}

func (self *SGuest) isGpu() bool {
	return len(self.GetIsolatedDevices()) != 0
}

func (self *SGuest) GetIsolatedDevices() []SIsolatedDevice {
	return IsolatedDeviceManager.findAttachedDevicesOfGuest(self)
}

func (self *SGuest) syncWithCloudVM(ctx context.Context, userCred mcclient.TokenCredential, host *SHost, extVM cloudprovider.ICloudVM, projectId string, projectSync bool) error {
	recycle := false

	if self.IsPrepaidRecycle() {
		recycle = true
	}

	metaData := extVM.GetMetadata()
	diff, err := GuestManager.TableSpec().Update(self, func() error {
		extVM.Refresh()
		// self.Name = extVM.GetName()
		self.Status = extVM.GetStatus()
		self.VcpuCount = extVM.GetVcpuCount()
		self.BootOrder = extVM.GetBootOrder()
		self.Vga = extVM.GetVga()
		self.Vdi = extVM.GetVdi()
		self.OsType = extVM.GetOSType()
		self.Bios = extVM.GetBios()
		self.Machine = extVM.GetMachine()
		if !recycle {
			self.HostId = host.Id
		}

		metaData := extVM.GetMetadata()
		instanceType := extVM.GetInstanceType()

		if len(instanceType) > 0 {
			self.InstanceType = instanceType
		}

		if extVM.GetHypervisor() == HYPERVISOR_AWS {
			sku, err := ServerSkuManager.FetchSkuByNameAndHypervisor(instanceType, extVM.GetHypervisor(), false)
			if err == nil {
				self.VmemSize = sku.MemorySizeMB
			} else {
				self.VmemSize = extVM.GetVmemSizeMB()
			}
		} else {
			self.VmemSize = extVM.GetVmemSizeMB()
		}

		if projectSync && len(projectId) > 0 {
			self.ProjectId = projectId
		}

		self.Hypervisor = extVM.GetHypervisor()

		self.IsEmulated = extVM.IsEmulated()

		if !recycle {
			self.BillingType = extVM.GetBillingType()
			self.ExpiredAt = extVM.GetExpiredAt()
		}

		if metaData != nil && metaData.Contains("secgroupIds") {
			secgroupIds := []string{}
			if err := metaData.Unmarshal(&secgroupIds, "secgroupIds"); err == nil {
				for _, secgroupId := range secgroupIds {
					secgrp, err := SecurityGroupManager.FetchByExternalId(secgroupId)
					if err != nil {
						log.Errorf("Failed find secgroup %s for guest %s error: %v", secgroupId, self.Name, err)
						continue
					}
					secgroup := secgrp.(*SSecurityGroup)
					if len(self.SecgrpId) == 0 {
						self.SecgrpId = secgroup.Id
					} else {
						if _, err := GuestsecgroupManager.newGuestSecgroup(ctx, userCred, self, secgroup); err != nil {
							log.Errorf("failed to bind secgroup %s for guest %s error: %v", secgroup.Name, self.Name, err)
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Errorf("%s", err)
		return err
	}
	if diff != nil {
		diffStr := sqlchemy.UpdateDiffString(diff)
		if len(diffStr) > 0 {
			db.OpsLog.LogEvent(self, db.ACT_UPDATE, diffStr, userCred)
		}
	}
	if metaData != nil {
		meta := make(map[string]string, 0)
		if err := metaData.Unmarshal(meta); err != nil {
			log.Errorf("Get VM Metadata error: %v", err)
		} else {
			for key, value := range meta {
				if err := self.SetMetadata(ctx, key, value, userCred); err != nil {
					log.Errorf("set guest %s mata %s => %s error: %v", self.Name, key, value, err)
				}
			}
		}
	}

	if recycle {
		vhost := self.GetHost()
		err = vhost.syncWithCloudPrepaidVM(extVM, host, projectSync)
		if err != nil {
			return err
		}
	}

	return nil
}

func (manager *SGuestManager) newCloudVM(ctx context.Context, userCred mcclient.TokenCredential, host *SHost, extVM cloudprovider.ICloudVM, projectId string) (*SGuest, error) {

	guest := SGuest{}
	guest.SetModelManager(manager)

	guest.Status = extVM.GetStatus()
	guest.ExternalId = extVM.GetGlobalId()
	guest.Name = extVM.GetName()
	guest.VcpuCount = extVM.GetVcpuCount()
	guest.BootOrder = extVM.GetBootOrder()
	guest.Vga = extVM.GetVga()
	guest.Vdi = extVM.GetVdi()
	guest.OsType = extVM.GetOSType()
	guest.Bios = extVM.GetBios()
	guest.Machine = extVM.GetMachine()
	guest.Hypervisor = extVM.GetHypervisor()

	guest.IsEmulated = extVM.IsEmulated()

	guest.BillingType = extVM.GetBillingType()
	guest.ExpiredAt = extVM.GetExpiredAt()

	guest.HostId = host.Id

	metaData := extVM.GetMetadata()
	instanceType := extVM.GetInstanceType()

	/*zoneExtId, err := metaData.GetString("zone_ext_id")
	if err != nil {
		log.Errorf("get zone external id fail %s", err)
	}

	isku, err := ServerSkuManager.FetchByZoneExtId(zoneExtId, instanceType)
	if err != nil {
		log.Errorf("get sku zone %s instance type %s fail %s", zoneExtId, instanceType, err)
	} else {
		guest.SkuId = isku.GetId()
	}*/

	if len(instanceType) > 0 {
		guest.InstanceType = instanceType
	}

	if extVM.GetHypervisor() == HYPERVISOR_AWS {
		sku, err := ServerSkuManager.FetchSkuByNameAndHypervisor(instanceType, extVM.GetHypervisor(), false)
		if err == nil {
			guest.VmemSize = sku.MemorySizeMB
		} else {
			guest.VmemSize = extVM.GetVmemSizeMB()
		}
	} else {
		guest.VmemSize = extVM.GetVmemSizeMB()
	}

	guest.ProjectId = userCred.GetProjectId()
	if len(projectId) > 0 {
		guest.ProjectId = projectId
	}

	extraSecgroups := []*SSecurityGroup{}
	if metaData != nil && metaData.Contains("secgroupIds") {
		secgroupIds := []string{}
		if err := metaData.Unmarshal(&secgroupIds, "secgroupIds"); err == nil {
			for _, secgroupId := range secgroupIds {
				secgrp, err := SecurityGroupManager.FetchByExternalId(secgroupId)
				if err != nil {
					log.Errorf("Failed find secgroup %s for guest %s error: %v", secgroupId, guest.Name, err)
					continue
				}
				secgroup := secgrp.(*SSecurityGroup)
				if len(guest.SecgrpId) == 0 {
					guest.SecgrpId = secgroup.Id
				} else {
					extraSecgroups = append(extraSecgroups, secgroup)
				}
			}
		}
	}

	err := manager.TableSpec().Insert(&guest)
	if err != nil {
		log.Errorf("Insert fail %s", err)
		return nil, err
	}

	for _, secgroup := range extraSecgroups {
		if _, err := GuestsecgroupManager.newGuestSecgroup(ctx, userCred, &guest, secgroup); err != nil {
			log.Errorf("failed to bind secgroup %s for guest %s error: %v", secgroup.Name, guest.Name, err)
		}
	}

	if metaData != nil {
		meta := make(map[string]string, 0)
		if err := metaData.Unmarshal(meta); err != nil {
			log.Errorf("Get VM Metadata error: %v", err)
		} else {
			for key, value := range meta {
				if err := guest.SetMetadata(ctx, key, value, userCred); err != nil {
					log.Errorf("set guest %s mata %s => %s error: %v", guest.Name, key, value, err)
				}
			}
		}
	}

	db.OpsLog.LogEvent(&guest, db.ACT_SYNC_CLOUD_SERVER, guest.GetShortDesc(), userCred)
	return &guest, nil
}

func (manager *SGuestManager) TotalCount(
	projectId string, rangeObj db.IStandaloneModel,
	status []string, hypervisors []string,
	includeSystem bool, pendingDelete bool,
	hostTypes []string, resourceTypes []string, providers []string,
) SGuestCountStat {
	return totalGuestResourceCount(projectId, rangeObj, status, hypervisors, includeSystem, pendingDelete, hostTypes, resourceTypes, providers)
}

func (self *SGuest) detachNetwork(ctx context.Context, userCred mcclient.TokenCredential, network *SNetwork, reserve bool, deploy bool) error {
	// Portmaps.delete_guest_network_portmaps(self, user_cred,
	//                                                    network_id=net.id)
	err := GuestnetworkManager.DeleteGuestNics(ctx, self, userCred, network, reserve)
	if err != nil {
		return err
	}
	host := self.GetHost()
	if host != nil {
		host.ClearSchedDescCache() // ignore error
	}
	if deploy {
		self.StartGuestDeployTask(ctx, userCred, nil, "deploy", "")
	}
	return nil
}

func (self *SGuest) isAttach2Network(net *SNetwork) bool {
	q := GuestnetworkManager.Query()
	q = q.Equals("guest_id", self.Id).Equals("network_id", net.Id)
	return q.Count() > 0
}

func (self *SGuest) getMaxNicIndex() int8 {
	nics := self.GetNetworks()
	return int8(len(nics))
}

func (self *SGuest) setOSProfile(ctx context.Context, userCred mcclient.TokenCredential, profile jsonutils.JSONObject) error {
	return self.SetMetadata(ctx, "__os_profile__", profile, userCred)
}

func (self *SGuest) getOSProfile() osprofile.SOSProfile {
	osName := self.GetOS()
	osProf := osprofile.GetOSProfile(osName, self.Hypervisor)
	val := self.GetMetadata("__os_profile__", nil)
	if len(val) > 0 {
		jsonVal, _ := jsonutils.ParseString(val)
		if jsonVal != nil {
			jsonVal.Unmarshal(&osProf)
		}
	}
	return osProf
}

func (self *SGuest) Attach2Network(ctx context.Context, userCred mcclient.TokenCredential, network *SNetwork, pendingUsage quotas.IQuota,
	address string, mac string, driver string, bwLimit int, virtual bool, index int8, reserved bool, allocDir IPAddlocationDirection, requireDesignatedIP bool) error {
	if self.isAttach2Network(network) {
		return fmt.Errorf("Guest has been attached to network %s", network.Name)
	}
	if index < 0 {
		index = self.getMaxNicIndex()
	}
	if len(driver) == 0 {
		osProf := self.getOSProfile()
		driver = osProf.NetDriver
	}
	lockman.LockClass(ctx, QuotaManager, self.ProjectId)
	defer lockman.ReleaseClass(ctx, QuotaManager, self.ProjectId)

	guestnic, err := GuestnetworkManager.newGuestNetwork(ctx, userCred, self, network,
		index, address, mac, driver, bwLimit, virtual, reserved,
		allocDir, requireDesignatedIP)
	if err != nil {
		return err
	}
	network.updateDnsRecord(guestnic, true)
	network.updateGuestNetmap(guestnic)
	bwLimit = guestnic.getBandwidth()
	if pendingUsage != nil {
		cancelUsage := SQuota{}
		if network.IsExitNetwork() {
			cancelUsage.Eport = 1
			cancelUsage.Ebw = bwLimit
		} else {
			cancelUsage.Port = 1
			cancelUsage.Bw = bwLimit
		}
		err = QuotaManager.CancelPendingUsage(ctx, userCred, self.ProjectId, pendingUsage, &cancelUsage)
		if err != nil {
			return err
		}
	}
	notes := jsonutils.NewDict()
	if len(address) == 0 {
		address = guestnic.IpAddr
	}
	notes.Add(jsonutils.NewString(address), "ip_addr")
	db.OpsLog.LogAttachEvent(self, network, userCred, notes)
	return nil
}

type sRemoveGuestnic struct {
	nic     *SGuestnetwork
	reserve bool
}

type sAddGuestnic struct {
	nic     cloudprovider.ICloudNic
	net     *SNetwork
	reserve bool
}

func getCloudNicNetwork(vnic cloudprovider.ICloudNic, host *SHost) (*SNetwork, error) {
	vnet := vnic.GetINetwork()
	if vnet == nil {
		ip := vnic.GetIP()
		if len(ip) == 0 {
			return nil, fmt.Errorf("Cannot find inetwork for vnics %s %s", vnic.GetMAC(), vnic.GetIP())
		} else {
			// find network by IP
			return host.getNetworkOfIPOnHost(vnic.GetIP())
		}
	}
	localNetObj, err := NetworkManager.FetchByExternalId(vnet.GetGlobalId())
	if err != nil {
		return nil, fmt.Errorf("Cannot find network of external_id %s: %v", vnet.GetGlobalId(), err)
	}
	localNet := localNetObj.(*SNetwork)
	return localNet, nil
}

func (self *SGuest) SyncVMNics(ctx context.Context, userCred mcclient.TokenCredential, host *SHost, vnics []cloudprovider.ICloudNic) compare.SyncResult {
	result := compare.SyncResult{}

	guestnics := self.GetNetworks()
	removed := make([]sRemoveGuestnic, 0)
	adds := make([]sAddGuestnic, 0)

	for i := 0; i < len(guestnics) || i < len(vnics); i += 1 {
		if i < len(guestnics) && i < len(vnics) {
			localNet, err := getCloudNicNetwork(vnics[i], host)
			if err != nil {
				log.Errorf("%s", err)
				result.Error(err)
				return result
			}
			if guestnics[i].NetworkId == localNet.Id {
				if guestnics[i].MacAddr == vnics[i].GetMAC() {
					if guestnics[i].IpAddr == vnics[i].GetIP() { // nothing changes
						// do nothing
					} else if len(vnics[i].GetIP()) > 0 {
						// ip changed
						removed = append(removed, sRemoveGuestnic{nic: &guestnics[i]})
						adds = append(adds, sAddGuestnic{nic: vnics[i], net: localNet})
					} else {
						// do nothing
						// vm maybe turned off, ignore the case
					}
				} else {
					reserve := false
					if len(guestnics[i].IpAddr) > 0 && guestnics[i].IpAddr == vnics[i].GetIP() {
						// mac changed
						reserve = true
					}
					removed = append(removed, sRemoveGuestnic{nic: &guestnics[i], reserve: reserve})
					adds = append(adds, sAddGuestnic{nic: vnics[i], net: localNet, reserve: reserve})
				}
			} else {
				removed = append(removed, sRemoveGuestnic{nic: &guestnics[i]})
				adds = append(adds, sAddGuestnic{nic: vnics[i], net: localNet})
			}
		} else if i < len(guestnics) {
			removed = append(removed, sRemoveGuestnic{nic: &guestnics[i]})
		} else if i < len(vnics) {
			localNet, err := getCloudNicNetwork(vnics[i], host)
			if err != nil {
				log.Errorf("%s", err) // ignore this case
			} else {
				adds = append(adds, sAddGuestnic{nic: vnics[i], net: localNet})
			}
		}
	}

	for _, remove := range removed {
		err := self.detachNetwork(ctx, userCred, remove.nic.GetNetwork(), remove.reserve, false)
		if err != nil {
			result.DeleteError(err)
		} else {
			result.Delete()
		}
	}

	for _, add := range adds {
		if len(add.nic.GetIP()) == 0 {
			continue // cannot determine which network it attached to
		}
		if add.net == nil {
			continue // cannot determine which network it attached to
		}
		// check if the IP has been occupied, if yes, release the IP
		gn, err := GuestnetworkManager.getGuestNicByIP(add.nic.GetIP(), add.net.Id)
		if err != nil {
			result.AddError(err)
			continue
		}
		if gn != nil {
			err = gn.Detach(ctx, userCred)
			if err != nil {
				result.AddError(err)
				continue
			}
		}
		err = self.Attach2Network(ctx, userCred, add.net, nil, add.nic.GetIP(),
			add.nic.GetMAC(), add.nic.GetDriver(), 0, false, -1, add.reserve, IPAllocationDefault, true)
		if err != nil {
			result.AddError(err)
		} else {
			result.Add()
		}
	}

	return result
}

func (self *SGuest) isAttach2Disk(disk *SDisk) bool {
	q := GuestdiskManager.Query().Equals("disk_id", disk.Id).Equals("guest_id", self.Id)
	return q.Count() > 0
}

func (self *SGuest) getMaxDiskIndex() int8 {
	guestdisks := self.GetDisks()
	return int8(len(guestdisks))
}

func (self *SGuest) AttachDisk(disk *SDisk, userCred mcclient.TokenCredential, driver string, cache string, mountpoint string) error {
	return self.attach2Disk(disk, userCred, driver, cache, mountpoint)
}

func (self *SGuest) attach2Disk(disk *SDisk, userCred mcclient.TokenCredential, driver string, cache string, mountpoint string) error {
	if self.isAttach2Disk(disk) {
		return fmt.Errorf("Guest has been attached to disk")
	}
	index := self.getMaxDiskIndex()
	if len(driver) == 0 {
		osProf := self.getOSProfile()
		driver = osProf.DiskDriver
	}
	guestdisk := SGuestdisk{}
	guestdisk.SetModelManager(GuestdiskManager)

	guestdisk.DiskId = disk.Id
	guestdisk.GuestId = self.Id
	guestdisk.Index = index
	err := guestdisk.DoSave(driver, cache, mountpoint)
	if err == nil {
		db.OpsLog.LogAttachEvent(self, disk, userCred, nil)
	}
	return err
}

type sSyncDiskPair struct {
	disk  *SDisk
	vdisk cloudprovider.ICloudDisk
}

func (self *SGuest) SyncVMDisks(ctx context.Context, userCred mcclient.TokenCredential, host *SHost, vdisks []cloudprovider.ICloudDisk, projectId string, projectSync bool) compare.SyncResult {
	result := compare.SyncResult{}

	newdisks := make([]sSyncDiskPair, 0)
	for i := 0; i < len(vdisks); i += 1 {
		if len(vdisks[i].GetGlobalId()) == 0 {
			continue
		}
		disk, err := DiskManager.syncCloudDisk(ctx, userCred, vdisks[i], i, projectId, projectSync)
		if err != nil {
			log.Errorf("syncCloudDisk error: %v", err)
			result.Error(err)
			return result
		}
		newdisks = append(newdisks, sSyncDiskPair{disk: disk, vdisk: vdisks[i]})
	}

	needRemoves := make([]SGuestdisk, 0)

	guestdisks := self.GetDisks()
	for i := 0; i < len(guestdisks); i += 1 {
		find := false
		for j := 0; j < len(newdisks); j += 1 {
			if newdisks[j].disk.Id == guestdisks[i].DiskId {
				find = true
				break
			}
		}
		if !find {
			needRemoves = append(needRemoves, guestdisks[i])
		}
	}

	needAdds := make([]sSyncDiskPair, 0)

	for i := 0; i < len(newdisks); i += 1 {
		find := false
		for j := 0; j < len(guestdisks); j += 1 {
			if newdisks[i].disk.Id == guestdisks[j].DiskId {
				find = true
				break
			}
		}
		if !find {
			needAdds = append(needAdds, newdisks[i])
		}
	}

	for i := 0; i < len(needRemoves); i += 1 {
		err := needRemoves[i].Detach(ctx, userCred)
		if err != nil {
			result.DeleteError(err)
		} else {
			result.Delete()
		}
	}
	for i := 0; i < len(needAdds); i += 1 {
		vdisk := needAdds[i].vdisk
		err := self.attach2Disk(needAdds[i].disk, userCred, vdisk.GetDriver(), vdisk.GetCacheMode(), vdisk.GetMountpoint())
		if err != nil {
			log.Errorf("attach2Disk error: %v", err)
			result.AddError(err)
		} else {
			result.Add()
		}
	}
	return result
}

func filterGuestByRange(q *sqlchemy.SQuery, rangeObj db.IStandaloneModel, hostTypes []string, resourceTypes []string, providers []string) *sqlchemy.SQuery {
	hosts := HostManager.Query().SubQuery()

	q = q.Join(hosts, sqlchemy.Equals(hosts.Field("id"), q.Field("host_id")))
	q = q.Filter(sqlchemy.IsTrue(hosts.Field("enabled")))
	// q = q.Filter(sqlchemy.Equals(hosts.Field("host_status"), HOST_ONLINE))

	q = AttachUsageQuery(q, hosts, hostTypes, resourceTypes, providers, rangeObj)
	return q
}

type SGuestCountStat struct {
	TotalGuestCount       int
	TotalCpuCount         int
	TotalMemSize          int
	TotalDiskSize         int
	TotalIsolatedCount    int
	TotalBackupGuestCount int
	TotalBackupCpuCount   int
	TotalBackupMemSize    int
	TotalBackupDiskSize   int
}

func totalGuestResourceCount(
	projectId string,
	rangeObj db.IStandaloneModel,
	status []string,
	hypervisors []string,
	includeSystem bool,
	pendingDelete bool,
	hostTypes []string,
	resourceTypes []string,
	providers []string,
) SGuestCountStat {

	guestdisks := GuestdiskManager.Query().SubQuery()
	disks := DiskManager.Query().SubQuery()

	diskQuery := guestdisks.Query(guestdisks.Field("guest_id"), sqlchemy.SUM("guest_disk_size", disks.Field("disk_size")))
	diskQuery = diskQuery.Join(disks, sqlchemy.Equals(guestdisks.Field("disk_id"), disks.Field("id")))
	diskQuery = diskQuery.GroupBy(guestdisks.Field("guest_id"))
	diskSubQuery := diskQuery.SubQuery()

	backupDiskQuery := guestdisks.Query(guestdisks.Field("guest_id"), sqlchemy.SUM("guest_disk_size", disks.Field("disk_size")))
	backupDiskQuery = backupDiskQuery.LeftJoin(disks, sqlchemy.Equals(guestdisks.Field("disk_id"), disks.Field("id")))
	backupDiskQuery = backupDiskQuery.Filter(sqlchemy.IsNotEmpty(disks.Field("backup_storage_id")))
	backupDiskQuery = backupDiskQuery.GroupBy(guestdisks.Field("guest_id"))

	diskBackupSubQuery := backupDiskQuery.SubQuery()
	// diskBackupSubQuery := diskQuery.IsNotEmpty("backup_storage_id").SubQuery()

	isolated := IsolatedDeviceManager.Query().SubQuery()

	isoDevQuery := isolated.Query(isolated.Field("guest_id"), sqlchemy.COUNT("device_sum"))
	isoDevQuery = isoDevQuery.Filter(sqlchemy.IsNotNull(isolated.Field("guest_id")))
	isoDevQuery = isoDevQuery.GroupBy(isolated.Field("guest_id"))

	isoDevSubQuery := isoDevQuery.SubQuery()

	guests := GuestManager.Query().SubQuery()
	guestBackupSubQuery := GuestManager.Query(
		"id",
		"vcpu_count",
		"vmem_size",
	).IsNotEmpty("backup_host_id").SubQuery()

	q := guests.Query(sqlchemy.COUNT("total_guest_count"),
		sqlchemy.SUM("total_cpu_count", guests.Field("vcpu_count")),
		sqlchemy.SUM("total_mem_size", guests.Field("vmem_size")),
		sqlchemy.SUM("total_disk_size", diskSubQuery.Field("guest_disk_size")),
		sqlchemy.SUM("total_isolated_count", isoDevSubQuery.Field("device_sum")),
		sqlchemy.SUM("total_backup_disk_size", diskBackupSubQuery.Field("guest_disk_size")),
		sqlchemy.SUM("total_backup_cpu_count", guestBackupSubQuery.Field("vcpu_count")),
		sqlchemy.SUM("total_backup_mem_size", guestBackupSubQuery.Field("vmem_size")),
		sqlchemy.COUNT("total_backup_guest_count", guestBackupSubQuery.Field("id")),
	)

	q = q.LeftJoin(guestBackupSubQuery, sqlchemy.Equals(guestBackupSubQuery.Field("id"), guests.Field("id")))

	q = q.LeftJoin(diskSubQuery, sqlchemy.Equals(diskSubQuery.Field("guest_id"), guests.Field("id")))
	q = q.LeftJoin(diskBackupSubQuery, sqlchemy.Equals(diskBackupSubQuery.Field("guest_id"), guests.Field("id")))

	q = q.LeftJoin(isoDevSubQuery, sqlchemy.Equals(isoDevSubQuery.Field("guest_id"), guests.Field("id")))

	q = filterGuestByRange(q, rangeObj, hostTypes, resourceTypes, providers)

	if len(projectId) > 0 {
		q = q.Filter(sqlchemy.Equals(guests.Field("tenant_id"), projectId))
	}
	if len(status) > 0 {
		q = q.Filter(sqlchemy.In(guests.Field("status"), status))
	}
	if len(hypervisors) > 0 {
		q = q.Filter(sqlchemy.In(guests.Field("hypervisor"), hypervisors))
	}
	if !includeSystem {
		q = q.Filter(sqlchemy.OR(sqlchemy.IsNull(guests.Field("is_system")), sqlchemy.IsFalse(guests.Field("is_system"))))
	}
	if pendingDelete {
		q = q.Filter(sqlchemy.IsTrue(guests.Field("pending_deleted")))
	} else {
		q = q.Filter(sqlchemy.OR(sqlchemy.IsNull(guests.Field("pending_deleted")), sqlchemy.IsFalse(guests.Field("pending_deleted"))))
	}
	stat := SGuestCountStat{}
	row := q.Row()
	err := q.Row2Struct(row, &stat)
	if err != nil {
		log.Errorf("%s", err)
	}
	stat.TotalCpuCount += stat.TotalBackupCpuCount
	stat.TotalMemSize += stat.TotalBackupMemSize
	stat.TotalDiskSize += stat.TotalBackupDiskSize
	return stat
}

func (self *SGuest) getDefaultNetworkConfig() *SNetworkConfig {
	netConf := SNetworkConfig{}
	netConf.BwLimit = options.Options.DefaultBandwidth
	osProf := self.getOSProfile()
	netConf.Driver = osProf.NetDriver
	return &netConf
}

func (self *SGuest) CreateNetworksOnHost(ctx context.Context, userCred mcclient.TokenCredential, host *SHost, data *jsonutils.JSONDict, pendingUsage quotas.IQuota) error {
	netJsonArray := jsonutils.GetArrayOfPrefix(data, "net")
	/* idx := 0
	for idx = 0; data.Contains(fmt.Sprintf("net.%d", idx)); idx += 1 {
		netJson, err := data.Get(fmt.Sprintf("net.%d", idx))
		if err != nil {
			return err
		}
		if netJson == jsonutils.JSONNull {
			break
		}

	}*/
	if len(netJsonArray) == 0 {
		netConfig := self.getDefaultNetworkConfig()
		return self.attach2RandomNetwork(ctx, userCred, host, netConfig, pendingUsage)
	}
	for idx := 0; idx < len(netJsonArray); idx += 1 {
		netConfig, err := parseNetworkInfo(userCred, netJsonArray[idx])
		if err != nil {
			return err
		}
		err = self.attach2NetworkDesc(ctx, userCred, host, netConfig, pendingUsage)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *SGuest) attach2NetworkDesc(ctx context.Context, userCred mcclient.TokenCredential, host *SHost, netConfig *SNetworkConfig, pendingUsage quotas.IQuota) error {
	var err1, err2 error
	if len(netConfig.Network) > 0 {
		err1 = self.attach2NamedNetworkDesc(ctx, userCred, host, netConfig, pendingUsage)
		if err1 == nil {
			return nil
		}
	}
	err2 = self.attach2RandomNetwork(ctx, userCred, host, netConfig, pendingUsage)
	if err2 == nil {
		return nil
	}
	if err1 != nil {
		return fmt.Errorf("%s/%s", err1, err2)
	} else {
		return err2
	}
}

func (self *SGuest) attach2NamedNetworkDesc(ctx context.Context, userCred mcclient.TokenCredential, host *SHost, netConfig *SNetworkConfig, pendingUsage quotas.IQuota) error {
	driver := self.GetDriver()
	net, mac, idx, allocDir := driver.GetNamedNetworkConfiguration(self, userCred, host, netConfig)
	if net != nil {
		err := self.Attach2Network(ctx, userCred, net, pendingUsage, netConfig.Address, mac, netConfig.Driver, netConfig.BwLimit, netConfig.Vip, idx, netConfig.Reserved, allocDir, false)
		if err != nil {
			return err
		} else {
			return nil
		}
	} else {
		return fmt.Errorf("Network %s not available", netConfig.Network)
	}
}

func (self *SGuest) attach2RandomNetwork(ctx context.Context, userCred mcclient.TokenCredential, host *SHost, netConfig *SNetworkConfig, pendingUsage quotas.IQuota) error {
	driver := self.GetDriver()
	return driver.Attach2RandomNetwork(self, ctx, userCred, host, netConfig, pendingUsage)
}

func (self *SGuest) CreateDisksOnHost(ctx context.Context, userCred mcclient.TokenCredential, host *SHost,
	data *jsonutils.JSONDict, pendingUsage quotas.IQuota, inheritBilling bool) error {
	diskJsonArray := jsonutils.GetArrayOfPrefix(data, "disk")
	for idx := 0; idx < len(diskJsonArray); idx += 1 { // .Contains(fmt.Sprintf("disk.%d", idx)); idx += 1 {
		diskConfig, err := parseDiskInfo(ctx, userCred, diskJsonArray[idx])
		if err != nil {
			return err
		}
		disk, err := self.createDiskOnHost(ctx, userCred, host, diskConfig, pendingUsage, inheritBilling)
		if err != nil {
			return err
		}
		data.Add(jsonutils.NewString(disk.Id), fmt.Sprintf("disk.%d.id", idx))
		if len(diskConfig.SnapshotId) > 0 {
			data.Add(jsonutils.NewString(diskConfig.SnapshotId), fmt.Sprintf("disk.%d.snapshot", idx))
		}
	}
	return nil
}

func (self *SGuest) createDiskOnStorage(ctx context.Context, userCred mcclient.TokenCredential, storage *SStorage,
	diskConfig *SDiskConfig, pendingUsage quotas.IQuota, inheritBilling bool) (*SDisk, error) {
	lockman.LockObject(ctx, storage)
	defer lockman.ReleaseObject(ctx, storage)

	lockman.LockClass(ctx, QuotaManager, self.ProjectId)
	defer lockman.ReleaseClass(ctx, QuotaManager, self.ProjectId)

	diskName := fmt.Sprintf("vdisk_%s_%d", self.Name, time.Now().UnixNano())

	billingType := BILLING_TYPE_POSTPAID
	billingCycle := ""
	if inheritBilling {
		billingType = self.BillingType
		billingCycle = self.BillingCycle
	}

	autoDelete := false
	if storage.IsLocal() || billingType == BILLING_TYPE_PREPAID {
		autoDelete = true
	}
	disk, err := storage.createDisk(diskName, diskConfig, userCred, self.ProjectId, autoDelete, self.IsSystem,
		billingType, billingCycle)

	if err != nil {
		return nil, err
	}

	cancelUsage := SQuota{}
	cancelUsage.Storage = disk.DiskSize
	err = QuotaManager.CancelPendingUsage(ctx, userCred, self.ProjectId, pendingUsage, &cancelUsage)

	return disk, nil
}

func (self *SGuest) createDiskOnHost(ctx context.Context, userCred mcclient.TokenCredential, host *SHost,
	diskConfig *SDiskConfig, pendingUsage quotas.IQuota, inheritBilling bool) (*SDisk, error) {
	storage := self.GetDriver().ChooseHostStorage(host, diskConfig.Backend)
	if storage == nil {
		return nil, fmt.Errorf("No storage to create disk")
	}
	disk, err := self.createDiskOnStorage(ctx, userCred, storage, diskConfig, pendingUsage, inheritBilling)
	if err != nil {
		return nil, err
	}
	if len(self.BackupHostId) > 0 {
		backupHost := HostManager.FetchHostById(self.BackupHostId)
		backupStorage := self.GetDriver().ChooseHostStorage(backupHost, diskConfig.Backend)
		_, err = disk.GetModelManager().TableSpec().Update(disk, func() error {
			disk.BackupStorageId = backupStorage.Id
			return nil
		})
		if err != nil {
			log.Errorf("Disk save backup storage error")
			return disk, err
		}
	}
	err = self.attach2Disk(disk, userCred, diskConfig.Driver, diskConfig.Cache, diskConfig.Mountpoint)
	return disk, err
}

func (self *SGuest) CreateIsolatedDeviceOnHost(ctx context.Context, userCred mcclient.TokenCredential, host *SHost, data *jsonutils.JSONDict, pendingUsage quotas.IQuota) error {
	devJsonArray := jsonutils.GetArrayOfPrefix(data, "isolated_device")
	for idx := 0; idx < len(devJsonArray); idx += 1 { // .Contains(fmt.Sprintf("isolated_device.%d", idx)); idx += 1 {
		devConfig, err := IsolatedDeviceManager.parseDeviceInfo(userCred, devJsonArray[idx])
		if err != nil {
			return err
		}
		err = self.createIsolatedDeviceOnHost(ctx, userCred, host, devConfig, pendingUsage)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *SGuest) createIsolatedDeviceOnHost(ctx context.Context, userCred mcclient.TokenCredential, host *SHost, devConfig *SIsolatedDeviceConfig, pendingUsage quotas.IQuota) error {
	lockman.LockClass(ctx, QuotaManager, self.ProjectId)
	defer lockman.ReleaseClass(ctx, QuotaManager, self.ProjectId)

	err := IsolatedDeviceManager.attachHostDeviceToGuestByDesc(self, host, devConfig, userCred)
	if err != nil {
		return err
	}

	cancelUsage := SQuota{IsolatedDevice: 1}
	err = QuotaManager.CancelPendingUsage(ctx, userCred, self.ProjectId, pendingUsage, &cancelUsage)
	return err
}

func (self *SGuest) attachIsolatedDevice(userCred mcclient.TokenCredential, dev *SIsolatedDevice) error {
	if len(dev.GuestId) > 0 {
		return fmt.Errorf("Isolated device already attached to another guest: %s", dev.GuestId)
	}
	if dev.HostId != self.HostId {
		return fmt.Errorf("Isolated device and guest are not located in the same host")
	}
	_, err := IsolatedDeviceManager.TableSpec().Update(dev, func() error {
		dev.GuestId = self.Id
		return nil
	})
	if err != nil {
		return err
	}
	db.OpsLog.LogEvent(self, db.ACT_GUEST_ATTACH_ISOLATED_DEVICE, dev.GetShortDesc(), userCred)
	return nil
}

func (self *SGuest) JoinGroups(userCred mcclient.TokenCredential, params *jsonutils.JSONDict) {
	// TODO
}

type SGuestDiskCategory struct {
	Root *SDisk
	Swap []*SDisk
	Data []*SDisk
}

func (self *SGuest) CategorizeDisks() SGuestDiskCategory {
	diskCat := SGuestDiskCategory{}
	guestdisks := self.GetDisks()
	if guestdisks == nil {
		log.Errorf("no disk for this server!!!")
		return diskCat
	}
	for _, gd := range guestdisks {
		if diskCat.Root == nil {
			diskCat.Root = gd.GetDisk()
		} else {
			disk := gd.GetDisk()
			if disk.FsFormat == "swap" {
				diskCat.Swap = append(diskCat.Swap, disk)
			} else {
				diskCat.Data = append(diskCat.Data, disk)
			}
		}
	}
	return diskCat
}

type SGuestNicCategory struct {
	InternalNics []SGuestnetwork
	ExternalNics []SGuestnetwork
}

func (self *SGuest) CategorizeNics() SGuestNicCategory {
	netCat := SGuestNicCategory{}

	guestnics := self.GetNetworks()
	if guestnics == nil {
		log.Errorf("no nics for this server!!!")
		return netCat
	}

	for _, gn := range guestnics {
		if gn.IsExit() {
			netCat.ExternalNics = append(netCat.ExternalNics, gn)
		} else {
			netCat.InternalNics = append(netCat.InternalNics, gn)
		}
	}
	return netCat
}

func (self *SGuest) LeaveAllGroups(userCred mcclient.TokenCredential) {
	groupGuests := make([]SGroupguest, 0)
	q := GroupguestManager.Query()
	err := q.Filter(sqlchemy.Equals(q.Field("guest_id"), self.Id)).All(&groupGuests)
	if err != nil {
		log.Errorln(err.Error())
		return
	}
	for _, gg := range groupGuests {
		gg.Delete(context.Background(), userCred)
		var group SGroup
		gq := GroupManager.Query()
		err := gq.Filter(sqlchemy.Equals(gq.Field("id"), gg.SrvtagId)).First(&group)
		if err != nil {
			log.Errorln(err.Error())
			return
		}
		db.OpsLog.LogDetachEvent(self, &group, userCred, nil)
	}
}

func (self *SGuest) DetachAllNetworks(ctx context.Context, userCred mcclient.TokenCredential) error {
	// from clouds.models.portmaps import Portmaps
	// Portmaps.delete_guest_network_portmaps(self, user_cred)
	return GuestnetworkManager.DeleteGuestNics(ctx, self, userCred, nil, false)
}

func (self *SGuest) EjectIso(userCred mcclient.TokenCredential) bool {
	cdrom := self.getCdrom()
	if len(cdrom.ImageId) > 0 {
		imageId := cdrom.ImageId
		if cdrom.ejectIso() {
			db.OpsLog.LogEvent(self, db.ACT_ISO_DETACH, imageId, userCred)
			return true
		}
	}
	return false
}

func (self *SGuest) Delete(ctx context.Context, userCred mcclient.TokenCredential) error {
	// self.SVirtualResourceBase.Delete(ctx, userCred)
	// override
	log.Infof("guest delete do nothing")
	return nil
}

func (self *SGuest) RealDelete(ctx context.Context, userCred mcclient.TokenCredential) error {
	return self.SVirtualResourceBase.Delete(ctx, userCred)
}

func (self *SGuest) AllowDeleteItem(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) bool {
	overridePendingDelete := false
	purge := false
	if data != nil {
		overridePendingDelete = jsonutils.QueryBoolean(data, "override_pending_delete", false)
		purge = jsonutils.QueryBoolean(data, "purge", false)
	}
	if (overridePendingDelete || purge) && !db.IsAdminAllowDelete(userCred, self) {
		return false
	}
	return self.IsOwner(userCred) || db.IsAdminAllowDelete(userCred, self)
}

func (self *SGuest) CustomizeDelete(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) error {
	overridePendingDelete := false
	purge := false
	if query != nil {
		overridePendingDelete = jsonutils.QueryBoolean(query, "override_pending_delete", false)
		purge = jsonutils.QueryBoolean(query, "purge", false)
	}
	return self.StartDeleteGuestTask(ctx, userCred, "", purge, overridePendingDelete)
}

func (self *SGuest) DeleteAllDisksInDB(ctx context.Context, userCred mcclient.TokenCredential) error {
	for _, guestdisk := range self.GetDisks() {
		disk := guestdisk.GetDisk()
		err := guestdisk.Detach(ctx, userCred)
		if err != nil {
			return err
		}
		db.OpsLog.LogEvent(disk, db.ACT_DELETE, nil, userCred)
		db.OpsLog.LogEvent(disk, db.ACT_DELOCATE, nil, userCred)
		err = disk.RealDelete(ctx, userCred)
		if err != nil {
			return err
		}
	}
	return nil
}

type SDeployConfig struct {
	Path    string
	Action  string
	Content string
}

func (self *SGuest) GetDeployConfigOnHost(ctx context.Context, host *SHost, params *jsonutils.JSONDict) *jsonutils.JSONDict {
	config := jsonutils.NewDict()

	desc := self.GetDriver().GetJsonDescAtHost(ctx, self, host)
	config.Add(desc, "desc")

	deploys := make([]SDeployConfig, 0)
	for idx := 0; params.Contains(fmt.Sprintf("deploy.%d.path", idx)); idx += 1 {
		path, _ := params.GetString(fmt.Sprintf("deploy.%d.path", idx))
		action, _ := params.GetString(fmt.Sprintf("deploy.%d.action", idx))
		content, _ := params.GetString(fmt.Sprintf("deploy.%d.content", idx))
		deploys = append(deploys, SDeployConfig{Path: path, Action: action, Content: content})
	}

	if len(deploys) > 0 {
		config.Add(jsonutils.Marshal(deploys), "deploys")
	}

	deployAction, _ := params.GetString("deploy_action")
	if len(deployAction) == 0 {
		deployAction = "deploy"
	}

	// resetPasswd := true
	// if deployAction == "deploy" {
	resetPasswd := jsonutils.QueryBoolean(params, "reset_password", true)
	//}

	if resetPasswd {
		config.Add(jsonutils.JSONTrue, "reset_password")
		passwd, _ := params.GetString("password")
		if len(passwd) > 0 {
			config.Add(jsonutils.NewString(passwd), "password")
		}
		keypair := self.getKeypair()
		if keypair != nil {
			config.Add(jsonutils.NewString(keypair.PublicKey), "public_key")
		}
		deletePubKey, _ := params.GetString("delete_public_key")
		if len(deletePubKey) > 0 {
			config.Add(jsonutils.NewString(deletePubKey), "delete_public_key")
		}
	} else {
		config.Add(jsonutils.JSONFalse, "reset_password")
	}

	// add default public keys
	_, adminPubKey, err := sshkeys.GetSshAdminKeypair(ctx)
	if err != nil {
		log.Errorf("fail to get ssh admin public key %s", err)
	}

	_, projPubKey, err := sshkeys.GetSshProjectKeypair(ctx, self.ProjectId)

	if err != nil {
		log.Errorf("fail to get ssh project public key %s", err)
	}

	config.Add(jsonutils.NewString(adminPubKey), "admin_public_key")
	config.Add(jsonutils.NewString(projPubKey), "project_public_key")

	config.Add(jsonutils.NewString(deployAction), "action")

	onFinish := "shutdown"
	if jsonutils.QueryBoolean(params, "auto_start", false) || jsonutils.QueryBoolean(params, "restart", false) {
		onFinish = "none"
	} else if utils.IsInStringArray(self.Status, []string{VM_ADMIN}) {
		onFinish = "none"
	}

	config.Add(jsonutils.NewString(onFinish), "on_finish")

	return config
}

func (self *SGuest) getVga() string {
	if utils.IsInStringArray(self.Vga, []string{"cirrus", "vmware", "qxl"}) {
		return self.Vga
	}
	return "std"
}

func (self *SGuest) GetVdi() string {
	if utils.IsInStringArray(self.Vdi, []string{"vnc", "spice"}) {
		return self.Vdi
	}
	return "vnc"
}

func (self *SGuest) getMachine() string {
	if utils.IsInStringArray(self.Machine, []string{"pc", "q35"}) {
		return self.Machine
	}
	return "pc"
}

func (self *SGuest) getBios() string {
	if utils.IsInStringArray(self.Bios, []string{"BIOS", "UEFI"}) {
		return self.Bios
	}
	return "BIOS"
}

func (self *SGuest) getKvmOptions() string {
	return self.GetMetadata("kvm", nil)
}

func (self *SGuest) getExtraOptions() jsonutils.JSONObject {
	return self.GetMetadataJson("extra_options", nil)
}

func (self *SGuest) GetJsonDescAtHypervisor(ctx context.Context, host *SHost) *jsonutils.JSONDict {
	desc := jsonutils.NewDict()

	desc.Add(jsonutils.NewString(self.Name), "name")
	if len(self.Description) > 0 {
		desc.Add(jsonutils.NewString(self.Description), "description")
	}
	desc.Add(jsonutils.NewString(self.Id), "uuid")
	desc.Add(jsonutils.NewInt(int64(self.VmemSize)), "mem")
	desc.Add(jsonutils.NewInt(int64(self.VcpuCount)), "cpu")
	desc.Add(jsonutils.NewString(self.getVga()), "vga")
	desc.Add(jsonutils.NewString(self.GetVdi()), "vdi")
	desc.Add(jsonutils.NewString(self.getMachine()), "machine")
	desc.Add(jsonutils.NewString(self.getBios()), "bios")
	desc.Add(jsonutils.NewString(self.BootOrder), "boot_order")

	if len(self.BackupHostId) > 0 {
		if self.HostId == host.Id {
			desc.Set("is_master", jsonutils.JSONTrue)
			desc.Set("host_id", jsonutils.NewString(self.HostId))
		} else if self.BackupHostId == host.Id {
			desc.Set("is_slave", jsonutils.JSONTrue)
			desc.Set("host_id", jsonutils.NewString(self.BackupHostId))
		}
	}

	// isolated devices
	isolatedDevs := IsolatedDeviceManager.generateJsonDescForGuest(self)
	desc.Add(jsonutils.NewArray(isolatedDevs...), "isolated_devices")

	// nics, domain
	jsonNics := make([]jsonutils.JSONObject, 0)
	nics := self.GetNetworks()
	domain := options.Options.DNSDomain
	if nics != nil && len(nics) > 0 {
		for _, nic := range nics {
			nicDesc := nic.getJsonDescAtHost(host)
			jsonNics = append(jsonNics, nicDesc)
			nicDomain, _ := nicDesc.GetString("domain")
			if len(nicDomain) > 0 && len(domain) == 0 {
				domain = nicDomain
			}
		}
	}
	desc.Add(jsonutils.NewArray(jsonNics...), "nics")
	desc.Add(jsonutils.NewString(domain), "domain")

	// disks
	jsonDisks := make([]jsonutils.JSONObject, 0)
	disks := self.GetDisks()
	if disks != nil && len(disks) > 0 {
		for _, disk := range disks {
			diskDesc := disk.GetJsonDescAtHost(host)
			jsonDisks = append(jsonDisks, diskDesc)
		}
	}
	desc.Add(jsonutils.NewArray(jsonDisks...), "disks")

	// cdrom
	cdDesc := self.getCdrom().getJsonDesc()
	if cdDesc != nil {
		desc.Add(cdDesc, "cdrom")
	}

	// tenant
	tc, _ := self.GetTenantCache(ctx)
	if tc != nil {
		desc.Add(jsonutils.NewString(tc.GetName()), "tenant")
	}
	desc.Add(jsonutils.NewString(self.ProjectId), "tenant_id")

	// flavor
	// desc.Add(jsonuitls.NewString(self.getFlavorName()), "flavor")

	keypair := self.getKeypair()
	if keypair != nil {
		desc.Add(jsonutils.NewString(keypair.Name), "keypair")
		desc.Add(jsonutils.NewString(keypair.PublicKey), "pubkey")
	}

	netRoles := self.getNetworkRoles()
	if netRoles != nil && len(netRoles) > 0 {
		desc.Add(jsonutils.NewStringArray(netRoles), "network_roles")
	}

	secGrp := self.getSecgroup()
	if secGrp != nil {
		desc.Add(jsonutils.NewString(secGrp.Name), "secgroup")
	}

	if secgroups := self.getSecgroupJson(); len(secgroups) > 0 {
		desc.Add(jsonutils.NewArray(secgroups...), "secgroups")
	}

	/*
		TODO
		srs := self.getSecurityRuleSet()
		if srs.estimatedSinglePortRuleCount() <= options.FirewallFlowCountLimit {
	*/

	rules := self.getSecurityGroupsRules()
	if len(rules) > 0 {
		desc.Add(jsonutils.NewString(rules), "security_rules")
	}
	rules = self.getAdminSecurityRules()
	if len(rules) > 0 {
		desc.Add(jsonutils.NewString(rules), "admin_security_rules")
	}

	extraOptions := self.getExtraOptions()
	if extraOptions != nil {
		desc.Add(extraOptions, "extra_options")
	}

	kvmOptions := self.getKvmOptions()
	if len(kvmOptions) > 0 {
		desc.Add(jsonutils.NewString(kvmOptions), "kvm")
	}

	zone := self.getZone()
	if zone != nil {
		desc.Add(jsonutils.NewString(zone.Id), "zone_id")
		desc.Add(jsonutils.NewString(zone.Name), "zone")
	}

	os := self.GetOS()
	if len(os) > 0 {
		desc.Add(jsonutils.NewString(os), "os_name")
	}

	meta, _ := self.GetAllMetadata(nil)
	desc.Add(jsonutils.Marshal(meta), "metadata")

	userData := meta["user_data"]
	if len(userData) > 0 {
		desc.Add(jsonutils.NewString(userData), "user_data")
	}

	if self.PendingDeleted {
		desc.Add(jsonutils.JSONTrue, "pending_deleted")
	} else {
		desc.Add(jsonutils.JSONFalse, "pending_deleted")
	}

	return desc
}

func (self *SGuest) GetJsonDescAtBaremetal(ctx context.Context, host *SHost) *jsonutils.JSONDict {
	desc := jsonutils.NewDict()

	desc.Add(jsonutils.NewString(self.Name), "name")
	if len(self.Description) > 0 {
		desc.Add(jsonutils.NewString(self.Description), "description")
	}
	desc.Add(jsonutils.NewString(self.Id), "uuid")
	desc.Add(jsonutils.NewInt(int64(self.VmemSize)), "mem")
	desc.Add(jsonutils.NewInt(int64(self.VcpuCount)), "cpu")
	diskConf := host.getDiskConfig()
	if diskConf != nil {
		desc.Add(diskConf, "disk_config")
	}

	jsonNics := make([]jsonutils.JSONObject, 0)
	jsonStandbyNics := make([]jsonutils.JSONObject, 0)

	netifs := host.GetNetInterfaces()
	domain := options.Options.DNSDomain

	if netifs != nil && len(netifs) > 0 {
		for _, nic := range netifs {
			nicDesc := nic.getServerJsonDesc()
			if nicDesc.Contains("ip") {
				jsonNics = append(jsonNics, nicDesc)
				nicDomain, _ := nicDesc.GetString("domain")
				if len(nicDomain) > 0 && len(domain) == 0 {
					domain = nicDomain
				}
			} else {
				jsonStandbyNics = append(jsonStandbyNics, nicDesc)
			}
		}
	}
	desc.Add(jsonutils.NewArray(jsonNics...), "nics")
	desc.Add(jsonutils.NewArray(jsonStandbyNics...), "nics_standby")
	desc.Add(jsonutils.NewString(domain), "domain")

	jsonDisks := make([]jsonutils.JSONObject, 0)
	disks := self.GetDisks()
	if disks != nil && len(disks) > 0 {
		for _, disk := range disks {
			diskDesc := disk.GetJsonDescAtHost(host)
			jsonDisks = append(jsonDisks, diskDesc)
		}
	}
	desc.Add(jsonutils.NewArray(jsonDisks...), "disks")

	tc, _ := self.GetTenantCache(ctx)
	if tc != nil {
		desc.Add(jsonutils.NewString(tc.GetName()), "tenant")
	}

	desc.Add(jsonutils.NewString(self.ProjectId), "tenant_id")

	keypair := self.getKeypair()
	if keypair != nil {
		desc.Add(jsonutils.NewString(keypair.Name), "keypair")
		desc.Add(jsonutils.NewString(keypair.PublicKey), "pubkey")
	}

	netRoles := self.getNetworkRoles()
	if netRoles != nil && len(netRoles) > 0 {
		desc.Add(jsonutils.NewStringArray(netRoles), "network_roles")
	}

	rules := self.getSecurityGroupsRules()
	if len(rules) > 0 {
		desc.Add(jsonutils.NewString(rules), "security_rules")
	}
	rules = self.getAdminSecurityRules()
	if len(rules) > 0 {
		desc.Add(jsonutils.NewString(rules), "admin_security_rules")
	}

	zone := self.getZone()
	if zone != nil {
		desc.Add(jsonutils.NewString(zone.Id), "zone_id")
		desc.Add(jsonutils.NewString(zone.Name), "zone")
	}

	os := self.GetOS()
	if len(os) > 0 {
		desc.Add(jsonutils.NewString(os), "os_name")
	}

	meta, _ := self.GetAllMetadata(nil)
	desc.Add(jsonutils.Marshal(meta), "metadata")

	userData := meta["user_data"]
	if len(userData) > 0 {
		desc.Add(jsonutils.NewString(userData), "user_data")
	}

	if self.PendingDeleted {
		desc.Add(jsonutils.JSONTrue, "pending_deleted")
	} else {
		desc.Add(jsonutils.JSONFalse, "pending_deleted")
	}

	return desc
}

func (self *SGuest) getNetworkRoles() []string {
	key := db.Metadata.GetSysadminKey("network_role")
	roleStr := self.GetMetadata(key, auth.AdminCredential())
	if len(roleStr) > 0 {
		return strings.Split(roleStr, ",")
	}
	return nil
}

func (manager *SGuestManager) FetchGuestById(guestId string) *SGuest {
	guest, err := manager.FetchById(guestId)
	if err != nil {
		log.Errorf("FetchById fail %s", err)
		return nil
	}
	return guest.(*SGuest)
}

func (self *SGuest) GetSpec(checkStatus bool) *jsonutils.JSONDict {
	if checkStatus {
		if utils.IsInStringArray(self.Status, []string{VM_SCHEDULE_FAILED}) {
			return nil
		}
	}
	spec := jsonutils.NewDict()
	spec.Set("cpu", jsonutils.NewInt(int64(self.VcpuCount)))
	spec.Set("mem", jsonutils.NewInt(int64(self.VmemSize)))

	// get disk spec
	guestdisks := self.GetDisks()
	diskSpecs := jsonutils.NewArray()
	for _, guestdisk := range guestdisks {
		info := guestdisk.ToDiskInfo()
		diskSpec := jsonutils.NewDict()
		diskSpec.Set("size", jsonutils.NewInt(info.Size))
		diskSpec.Set("backend", jsonutils.NewString(info.Backend))
		diskSpec.Set("medium_type", jsonutils.NewString(info.MediumType))
		diskSpec.Set("disk_type", jsonutils.NewString(info.DiskType))
		diskSpecs.Add(diskSpec)
	}
	spec.Set("disk", diskSpecs)

	// get nic spec
	guestnics := self.GetNetworks()
	nicSpecs := jsonutils.NewArray()
	for _, guestnic := range guestnics {
		nicSpec := jsonutils.NewDict()
		nicSpec.Set("bandwidth", jsonutils.NewInt(int64(guestnic.getBandwidth())))
		t := "int"
		if guestnic.IsExit() {
			t = "ext"
		}
		nicSpec.Set("type", jsonutils.NewString(t))
		nicSpecs.Add(nicSpec)
	}
	spec.Set("nic", nicSpecs)

	// get isolate device spec
	guestgpus := self.GetIsolatedDevices()
	gpuSpecs := jsonutils.NewArray()
	for _, guestgpu := range guestgpus {
		if strings.HasPrefix(guestgpu.DevType, "GPU") {
			gs := guestgpu.GetSpec(false)
			if gs != nil {
				gpuSpecs.Add(gs)
			}
		}
	}
	spec.Set("gpu", gpuSpecs)
	return spec
}

func (manager *SGuestManager) GetSpecIdent(spec *jsonutils.JSONDict) []string {
	cpuCount, _ := spec.Int("cpu")
	memSize, _ := spec.Int("mem")
	memSizeMB, _ := utils.GetSizeMB(fmt.Sprintf("%d", memSize), "M")
	specKeys := []string{
		fmt.Sprintf("cpu:%d", cpuCount),
		fmt.Sprintf("mem:%dM", memSizeMB),
	}

	countKey := func(kf func(*jsonutils.JSONDict) string, dataArray jsonutils.JSONObject) map[string]int64 {
		countMap := make(map[string]int64)
		datas, _ := dataArray.GetArray()
		for _, data := range datas {
			key := kf(data.(*jsonutils.JSONDict))
			if count, ok := countMap[key]; !ok {
				countMap[key] = 1
			} else {
				count++
				countMap[key] = count
			}
		}
		return countMap
	}

	kfuncs := map[string]func(*jsonutils.JSONDict) string{
		"disk": func(data *jsonutils.JSONDict) string {
			backend, _ := data.GetString("backend")
			mediumType, _ := data.GetString("medium_type")
			size, _ := data.Int("size")
			sizeGB, _ := utils.GetSizeGB(fmt.Sprintf("%d", size), "M")
			return fmt.Sprintf("disk:%s_%s_%dG", backend, mediumType, sizeGB)
		},
		"nic": func(data *jsonutils.JSONDict) string {
			typ, _ := data.GetString("type")
			bw, _ := data.Int("bandwidth")
			return fmt.Sprintf("nic:%s_%dM", typ, bw)
		},
		"gpu": func(data *jsonutils.JSONDict) string {
			vendor, _ := data.GetString("vendor")
			model, _ := data.GetString("model")
			return fmt.Sprintf("gpu:%s_%s", vendor, model)
		},
	}

	for sKey, kf := range kfuncs {
		sArrary, err := spec.Get(sKey)
		if err != nil {
			log.Errorf("Get key %s array error: %v", sKey, err)
			continue
		}
		for key, count := range countKey(kf, sArrary) {
			specKeys = append(specKeys, fmt.Sprintf("%sx%d", key, count))
		}
	}
	return specKeys
}

func (self *SGuest) GetTemplateId() string {
	guestdisks := self.GetDisks()
	for _, guestdisk := range guestdisks {
		disk := guestdisk.GetDisk()
		if disk != nil {
			templateId := disk.GetTemplateId()
			if len(templateId) > 0 {
				return templateId
			}
		}
	}
	return ""
}

func (self *SGuest) GetShortDesc() *jsonutils.JSONDict {
	desc := self.SStandaloneResourceBase.GetShortDesc()
	desc.Set("mem", jsonutils.NewInt(int64(self.VmemSize)))
	desc.Set("cpu", jsonutils.NewInt(int64(self.VcpuCount)))

	address := jsonutils.NewString(strings.Join(self.getRealIPs(), ","))
	desc.Set("ip_addr", address)

	if len(self.OsType) > 0 {
		desc.Add(jsonutils.NewString(self.OsType), "os_type")
	}
	if osDist := self.GetMetadata("os_distribution", nil); len(osDist) > 0 {
		desc.Add(jsonutils.NewString(osDist), "os_distribution")
	}
	if osVer := self.GetMetadata("os_version", nil); len(osVer) > 0 {
		desc.Add(jsonutils.NewString(osVer), "os_version")
	}

	templateId := self.GetTemplateId()
	if len(templateId) > 0 {
		desc.Set("template_id", jsonutils.NewString(templateId))
	}
	extBw := self.getBandwidth(true)
	intBw := self.getBandwidth(false)
	if extBw > 0 {
		desc.Set("ext_bandwidth", jsonutils.NewInt(int64(extBw)))
	}
	if intBw > 0 {
		desc.Set("int_bandwidth", jsonutils.NewInt(int64(intBw)))
	}

	if len(self.OsType) > 0 {
		desc.Add(jsonutils.NewString(self.OsType), "os_type")
	}

	if len(self.ExternalId) > 0 {
		desc.Add(jsonutils.NewString(self.ExternalId), "externalId")
	}

	desc.Set("hypervisor", jsonutils.NewString(self.GetHypervisor()))

	host := self.GetHost()

	spec := self.GetSpec(false)
	if self.GetHypervisor() == HYPERVISOR_BAREMETAL {
		if host != nil {
			hostSpec := host.GetSpec(false)
			hostSpecIdent := HostManager.GetSpecIdent(hostSpec)
			spec.Set("host_spec", jsonutils.NewString(strings.Join(hostSpecIdent, "/")))
		}
	}
	if spec != nil {
		desc.Update(spec)
	}

	var billingInfo SCloudBillingInfo

	if host != nil {
		billingInfo = host.getCloudBillingInfo()
	}

	if priceKey := self.GetMetadata("price_key", nil); len(priceKey) > 0 {
		billingInfo.PriceKey = priceKey
	}

	self.FetchCloudBillingInfo(&billingInfo)

	desc.Update(jsonutils.Marshal(billingInfo))

	return desc
}

func (self *SGuest) saveOsType(osType string) error {
	_, err := self.GetModelManager().TableSpec().Update(self, func() error {
		self.OsType = osType
		return nil
	})
	return err
}

func (self *SGuest) SaveDeployInfo(ctx context.Context, userCred mcclient.TokenCredential, data jsonutils.JSONObject) {
	info := make(map[string]interface{})
	if data.Contains("os") {
		osName, _ := data.GetString("os")
		self.saveOsType(osName)
		info["os_name"] = osName
	}
	if data.Contains("account") {
		account, _ := data.GetString("account")
		info["login_account"] = account
		if data.Contains("key") {
			key, _ := data.GetString("key")
			info["login_key"] = key
			info["login_key_timestamp"] = timeutils.UtcNow()
		} else {
			info["login_key"] = "none"
			info["login_key_timestamp"] = "none"
		}
	}
	if data.Contains("distro") {
		dist, _ := data.GetString("distro")
		info["os_distribution"] = dist
	}
	if data.Contains("version") {
		ver, _ := data.GetString("version")
		info["os_version"] = ver
	}
	if data.Contains("arch") {
		arch, _ := data.GetString("arch")
		info["os_arch"] = arch
	}
	if data.Contains("language") {
		lang, _ := data.GetString("language")
		info["os_language"] = lang
	}
	self.SetAllMetadata(ctx, info, userCred)
}

func (self *SGuest) isAllDisksReady() bool {
	ready := true
	disks := self.GetDisks()
	if disks == nil || len(disks) == 0 {
		log.Errorf("No valid disks")
		return false
	}
	for i := 0; i < len(disks); i += 1 {
		disk := disks[i].GetDisk()
		if !(disk.isReady() || disk.Status == DISK_START_MIGRATE) {
			ready = false
			break
		}
	}
	return ready
}

func (self *SGuest) GetKeypairPublicKey() string {
	keypair := self.getKeypair()
	if keypair != nil {
		return keypair.PublicKey
	}
	return ""
}

func (manager *SGuestManager) GetIpInProjectWithName(projectId, name string, isExitOnly bool) []string {
	guestnics := GuestnetworkManager.Query().SubQuery()
	guests := manager.Query().SubQuery()
	networks := NetworkManager.Query().SubQuery()
	q := guestnics.Query(guestnics.Field("ip_addr")).Join(guests,
		sqlchemy.AND(
			sqlchemy.Equals(guests.Field("id"), guestnics.Field("guest_id")),
			sqlchemy.OR(sqlchemy.IsNull(guests.Field("pending_deleted")),
				sqlchemy.IsFalse(guests.Field("pending_deleted"))))).
		Join(networks, sqlchemy.Equals(networks.Field("id"), guestnics.Field("network_id"))).
		Filter(sqlchemy.Equals(guests.Field("name"), name)).
		Filter(sqlchemy.NotEquals(guestnics.Field("ip_addr"), "")).
		Filter(sqlchemy.IsNotNull(guestnics.Field("ip_addr"))).
		Filter(sqlchemy.IsNotNull(networks.Field("guest_gateway")))
	ips := make([]string, 0)
	rows, err := q.Rows()
	if err != nil {
		log.Errorf("Get guest ip with name query err: %v", err)
		return ips
	}
	for rows.Next() {
		var ip string
		err = rows.Scan(&ip)
		if err != nil {
			log.Errorf("Get guest ip with name scan err: %v", err)
			return ips
		}
		ips = append(ips, ip)
	}
	return manager.getIpsByExit(ips, isExitOnly)
}

func (manager *SGuestManager) getIpsByExit(ips []string, isExitOnly bool) []string {
	intRet := make([]string, 0)
	extRet := make([]string, 0)
	for _, ip := range ips {
		addr, _ := netutils.NewIPV4Addr(ip)
		if netutils.IsExitAddress(addr) {
			extRet = append(extRet, ip)
			continue
		}
		intRet = append(intRet, ip)
	}
	if isExitOnly {
		return extRet
	} else if len(intRet) > 0 {
		return intRet
	}
	return extRet
}

func (manager *SGuestManager) getExpiredPendingDeleteGuests() []SGuest {
	deadline := time.Now().Add(time.Duration(options.Options.PendingDeleteExpireSeconds*-1) * time.Second)

	q := manager.Query()
	q = q.IsTrue("pending_deleted").LT("pending_deleted_at", deadline).Limit(options.Options.PendingDeleteMaxCleanBatchSize)

	guests := make([]SGuest, 0)
	err := db.FetchModelObjects(GuestManager, q, &guests)
	if err != nil {
		log.Errorf("fetch guests error %s", err)
		return nil
	}

	return guests
}

func (manager *SGuestManager) CleanPendingDeleteServers(ctx context.Context, userCred mcclient.TokenCredential, isStart bool) {
	guests := manager.getExpiredPendingDeleteGuests()
	if guests == nil {
		return
	}
	for i := 0; i < len(guests); i += 1 {
		guests[i].StartDeleteGuestTask(ctx, userCred, "", false, true)
	}
}

func (manager *SGuestManager) getExpiredPrepaidGuests() []SGuest {
	deadline := time.Now().Add(time.Duration(options.Options.PrepaidExpireCheckSeconds*-1) * time.Second)

	q := manager.Query()
	q = q.Equals("billing_type", BILLING_TYPE_PREPAID).LT("expired_at", deadline).Limit(options.Options.ExpiredPrepaidMaxCleanBatchSize)

	guests := make([]SGuest, 0)
	err := db.FetchModelObjects(GuestManager, q, &guests)
	if err != nil {
		log.Errorf("fetch guests error %s", err)
		return nil
	}

	return guests
}

func (manager *SGuestManager) DeleteExpiredPrepaidServers(ctx context.Context, userCred mcclient.TokenCredential, isStart bool) {
	guests := manager.getExpiredPrepaidGuests()
	if guests == nil {
		return
	}
	for i := 0; i < len(guests); i += 1 {
		guests[i].StartDeleteGuestTask(ctx, userCred, "", false, true)
	}
}

func (self *SGuest) GetEip() (*SElasticip, error) {
	return ElasticipManager.getEipForInstance("server", self.Id)
}

func (self *SGuest) GetRealIps() []string {
	return self.getRealIPs()
}

func (self *SGuest) SyncVMEip(ctx context.Context, userCred mcclient.TokenCredential, extEip cloudprovider.ICloudEIP, projectId string) compare.SyncResult {
	result := compare.SyncResult{}

	eip, err := self.GetEip()
	if err != nil {
		result.Error(fmt.Errorf("getEip error %s", err))
		return result
	}

	if eip == nil && extEip == nil {
		// do nothing
	} else if eip == nil && extEip != nil {
		// add
		neip, err := ElasticipManager.getEipByExtEip(userCred, extEip, self.getRegion(), projectId)
		if err != nil {
			log.Errorf("getEipByExtEip error %v", err)
			result.AddError(err)
		} else {
			err = neip.AssociateVM(userCred, self)
			if err != nil {
				log.Errorf("AssociateVM error %v", err)
				result.AddError(err)
			} else {
				result.Add()
			}
		}
	} else if eip != nil && extEip == nil {
		// remove
		err = eip.Dissociate(ctx, userCred)
		if err != nil {
			result.DeleteError(err)
		} else {
			result.Delete()
		}
	} else {
		// sync
		if eip.IpAddr != extEip.GetIpAddr() {
			// remove then add
			err = eip.Dissociate(ctx, userCred)
			if err != nil {
				// fail to remove
				result.DeleteError(err)
			} else {
				result.Delete()
				neip, err := ElasticipManager.getEipByExtEip(userCred, extEip, self.getRegion(), projectId)
				if err != nil {
					result.AddError(err)
				} else {
					err = neip.AssociateVM(userCred, self)
					if err != nil {
						result.AddError(err)
					} else {
						result.Add()
					}
				}
			}
		} else {
			// do nothing
			err := eip.SyncWithCloudEip(userCred, extEip, projectId, false)
			if err != nil {
				result.UpdateError(err)
			} else {
				result.Update()
			}
		}
	}

	return result
}

func (self *SGuest) GetIVM() (cloudprovider.ICloudVM, error) {
	if len(self.ExternalId) == 0 {
		msg := fmt.Sprintf("GetIVM: not managed by a provider")
		log.Errorf(msg)
		return nil, fmt.Errorf(msg)
	}
	host := self.GetHost()
	if host == nil {
		msg := fmt.Sprintf("GetIVM: No valid host")
		log.Errorf(msg)
		return nil, fmt.Errorf(msg)
	}
	ihost, err := host.GetIHost()
	if err != nil {
		msg := fmt.Sprintf("GetIVM: getihost fail %s", err)
		log.Errorf(msg)
		return nil, fmt.Errorf(msg)
	}
	return ihost.GetIVMById(self.ExternalId)
}

func (self *SGuest) DeleteEip(ctx context.Context, userCred mcclient.TokenCredential) error {
	eip, err := self.GetEip()
	if err != nil {
		log.Errorf("Delete eip fail for get Eip %s", err)
		return err
	}
	if eip == nil {
		return nil
	}
	if eip.Mode == EIP_MODE_INSTANCE_PUBLICIP {
		err = eip.RealDelete(ctx, userCred)
		if err != nil {
			log.Errorf("Delete eip on delete server fail %s", err)
			return err
		}
	} else {
		err = eip.Dissociate(ctx, userCred)
		if err != nil {
			log.Errorf("Dissociate eip on delete server fail %s", err)
			return err
		}
	}
	return nil
}

func (self *SGuest) SetDisableDelete(val bool) error {
	_, err := self.GetModelManager().TableSpec().Update(self, func() error {
		if val {
			self.DisableDelete = tristate.True
		} else {
			self.DisableDelete = tristate.False
		}
		return nil
	})
	return err
}

func (self *SGuest) getDefaultStorageType() string {
	diskCat := self.CategorizeDisks()
	if diskCat.Root != nil {
		rootStorage := diskCat.Root.GetStorage()
		if rootStorage != nil {
			return rootStorage.StorageType
		}
	}
	return STORAGE_LOCAL
}

func (self *SGuest) getSchedDesc() jsonutils.JSONObject {
	desc := jsonutils.NewDict()

	desc.Add(jsonutils.NewString(self.Id), "id")
	desc.Add(jsonutils.NewString(self.Name), "name")
	desc.Add(jsonutils.NewInt(int64(self.VmemSize)), "vmem_size")
	desc.Add(jsonutils.NewInt(int64(self.VcpuCount)), "vcpu_count")

	gds := self.GetDisks()
	if gds != nil {
		for i := 0; i < len(gds); i += 1 {
			desc.Add(jsonutils.Marshal(gds[i].ToDiskInfo()), fmt.Sprintf("disk.%d", i))
		}
	}

	gns := self.GetNetworks()
	if gns != nil {
		for i := 0; i < len(gns); i += 1 {
			desc.Add(jsonutils.NewString(fmt.Sprintf("%s:%s", gns[i].NetworkId, gns[i].IpAddr)), fmt.Sprintf("net.%d", i))
		}
	}

	if len(self.HostId) > 0 && regutils.MatchUUID(self.HostId) {
		desc.Add(jsonutils.NewString(self.HostId), "host_id")
	}

	desc.Add(jsonutils.NewString(self.ProjectId), "owner_tenant_id")
	desc.Add(jsonutils.NewString(self.GetHypervisor()), "hypervisor")

	return desc
}

func (self *SGuest) GetApptags() []string {
	tagsStr := self.GetMetadata("app_tags", nil)
	if len(tagsStr) > 0 {
		return strings.Split(tagsStr, ",")
	}
	return nil
}

func (self *SGuest) ToSchedDesc() *jsonutils.JSONDict {
	desc := jsonutils.NewDict()
	desc.Set("id", jsonutils.NewString(self.Id))
	desc.Set("name", jsonutils.NewString(self.Name))
	desc.Set("vmem_size", jsonutils.NewInt(int64(self.VmemSize)))
	desc.Set("vcpu_count", jsonutils.NewInt(int64(self.VcpuCount)))
	self.FillGroupSchedDesc(desc)
	self.FillDiskSchedDesc(desc)
	self.FillNetSchedDesc(desc)
	if len(self.HostId) > 0 && regutils.MatchUUID(self.HostId) {
		desc.Set("host_id", jsonutils.NewString(self.HostId))
	}
	desc.Set("owner_tenant_id", jsonutils.NewString(self.ProjectId))
	tags := self.GetApptags()
	for i := 0; i < len(tags); i++ {
		desc.Set(tags[i], jsonutils.JSONTrue)
	}
	desc.Set("hypervisor", jsonutils.NewString(self.GetHypervisor()))
	return desc
}

func (self *SGuest) FillGroupSchedDesc(desc *jsonutils.JSONDict) {
	groups := make([]SGroupguest, 0)
	err := GroupguestManager.Query().Equals("guest_id", self.Id).All(&groups)
	if err != nil {
		log.Errorln(err)
		return
	}
	for i := 0; i < len(groups); i++ {
		desc.Set(fmt.Sprintf("srvtag.%d", i),
			jsonutils.NewString(fmt.Sprintf("%s:%s", groups[i].SrvtagId, groups[i].Tag)))
	}
}

func (self *SGuest) FillDiskSchedDesc(desc *jsonutils.JSONDict) {
	guestDisks := make([]SGuestdisk, 0)
	err := GuestdiskManager.Query().Equals("guest_id", self.Id).All(&guestDisks)
	if err != nil {
		log.Errorln(err)
		return
	}
	for i := 0; i < len(guestDisks); i++ {
		desc.Set(fmt.Sprintf("disk.%d", i), jsonutils.Marshal(guestDisks[i].ToDiskInfo()))
	}
}

func (self *SGuest) FillNetSchedDesc(desc *jsonutils.JSONDict) {
	guestNetworks := make([]SGuestnetwork, 0)
	err := GuestnetworkManager.Query().Equals("guest_id", self.Id).All(&guestNetworks)
	if err != nil {
		log.Errorln(err)
		return
	}
	for i := 0; i < len(guestNetworks); i++ {
		desc.Set(fmt.Sprintf("net.%d", i),
			jsonutils.NewString(fmt.Sprintf("%s:%s",
				guestNetworks[i].NetworkId, guestNetworks[i].IpAddr)))
	}
}

func (self *SGuest) GuestDisksHasSnapshot() bool {
	guestDisks := self.GetDisks()
	for i := 0; i < len(guestDisks); i++ {
		if SnapshotManager.GetDiskSnapshotCount(guestDisks[i].DiskId) > 0 {
			return true
		}
	}
	return false
}

func (self *SGuest) OnScheduleToHost(ctx context.Context, userCred mcclient.TokenCredential, hostId string) error {
	err := self.SetHostId(hostId)
	if err != nil {
		return err
	}

	notes := jsonutils.NewDict()
	notes.Add(jsonutils.NewString(hostId), "host_id")
	db.OpsLog.LogEvent(self, db.ACT_SCHEDULE, notes, userCred)

	return nil
}
