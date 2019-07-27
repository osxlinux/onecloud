package aliyun

import (
	"fmt"

	api "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/cloudprovider"
	"yunion.io/x/onecloud/pkg/multicloud"

	"yunion.io/x/pkg/errors"
)

type SDatabasePrivileges struct {
	DatabasePrivilege []SDatabasePrivilege
}

type SDBInstanceAccount struct {
	multicloud.SDBInstanceAccountBase
	instance *SDBInstance

	AccountDescription string
	AccountName        string
	AccountStatus      string
	AccountType        string
	DBInstanceId       string
	DatabasePrivileges SDatabasePrivileges
	PrivExceeded       string
}

func (account *SDBInstanceAccount) GetId() string {
	return account.AccountName
}

func (account *SDBInstanceAccount) GetGlobalId() string {
	return account.AccountName
}

func (account *SDBInstanceAccount) GetName() string {
	return account.AccountName
}

func (account *SDBInstanceAccount) GetStatus() string {
	switch account.AccountStatus {
	case "Available":
		return api.DBINSTANCE_USER_AVAILABLE
	case "Unavailable":
		return api.DBINSTANCE_USER_UNAVAILABLE
	}
	return account.AccountStatus
}

func (account *SDBInstanceAccount) GetIDBInstanceAccountPrivileges() ([]cloudprovider.ICloudDBInstanceAccountPrivilege, error) {
	privileves := []cloudprovider.ICloudDBInstanceAccountPrivilege{}
	for i := 0; i < len(account.DatabasePrivileges.DatabasePrivilege); i++ {
		account.DatabasePrivileges.DatabasePrivilege[i].account = account
		privileves = append(privileves, &account.DatabasePrivileges.DatabasePrivilege[i])
	}
	return privileves, nil
}

func (region *SRegion) GetDBInstanceAccounts(instanceId string, offset int, limit int) ([]SDBInstanceAccount, int, error) {
	if limit > 50 || limit <= 0 {
		limit = 50
	}
	params := map[string]string{
		"RegionId":     region.RegionId,
		"PageSize":     fmt.Sprintf("%d", limit),
		"PageNumber":   fmt.Sprintf("%d", (offset/limit)+1),
		"DBInstanceId": instanceId,
	}
	body, err := region.rdsRequest("DescribeAccounts", params)
	if err != nil {
		return nil, 0, errors.Wrap(err, "DescribeAccounts")
	}
	accounts := []SDBInstanceAccount{}
	err = body.Unmarshal(&accounts, "Accounts", "DBInstanceAccount")
	if err != nil {
		return nil, 0, errors.Wrap(err, "Unmarshal")
	}
	total, _ := body.Int("TotalRecordCount")
	return accounts, int(total), nil
}