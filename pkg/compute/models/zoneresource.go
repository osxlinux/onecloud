// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"context"
	"database/sql"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"
	"yunion.io/x/pkg/util/reflectutils"
	"yunion.io/x/sqlchemy"

	api "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/cloudcommon/db"
	"yunion.io/x/onecloud/pkg/httperrors"
	"yunion.io/x/onecloud/pkg/mcclient"
	"yunion.io/x/onecloud/pkg/util/stringutils2"
)

type SZoneResourceBase struct {
	ZoneId string `width:"36" charset:"ascii" nullable:"true" list:"user" create:"optional" update:"user" json:"zone_id"`
}

type SZoneResourceBaseManager struct {
	SCloudregionResourceBaseManager
}

func ValidateZoneResourceInput(userCred mcclient.TokenCredential, query api.ZoneResourceInput) (*SZone, api.ZoneResourceInput, error) {
	zoneObj, err := ZoneManager.FetchByIdOrName(userCred, query.Zone)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, query, errors.Wrapf(httperrors.ErrResourceNotFound, "%s %s", ZoneManager.Keyword(), query.Zone)
		} else {
			return nil, query, errors.Wrap(err, "ZoneManager.FetchByIdOrName")
		}
	}
	query.Zone = zoneObj.GetId()
	return zoneObj.(*SZone), query, nil
}

func (self *SZoneResourceBase) GetZone() *SZone {
	return ZoneManager.FetchZoneById(self.ZoneId)
}

func (self *SZoneResourceBase) GetExtraDetails(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject) api.ZoneResourceInfo {
	return api.ZoneResourceInfo{}
}

func (manager *SZoneResourceBaseManager) FetchCustomizeColumns(
	ctx context.Context,
	userCred mcclient.TokenCredential,
	query jsonutils.JSONObject,
	objs []interface{},
	fields stringutils2.SSortedStrings,
	isList bool,
) []api.ZoneResourceInfo {
	rows := make([]api.ZoneResourceInfo, len(objs))
	zoneIds := make([]string, len(objs))
	for i := range objs {
		var base *SZoneResourceBase
		err := reflectutils.FindAnonymouStructPointer(objs[i], &base)
		if err != nil {
			log.Errorf("Cannot find SCloudregionResourceBase in object %s", objs[i])
			continue
		}
		zoneIds[i] = base.ZoneId
	}

	zones := make(map[string]SZone)
	err := db.FetchStandaloneObjectsByIds(ZoneManager, zoneIds, &zones)
	if err != nil {
		log.Errorf("FetchStandaloneObjectsByIds fail %s", err)
		return rows
	}

	regions := make([]interface{}, len(rows))
	for i := range rows {
		rows[i] = api.ZoneResourceInfo{}
		if _, ok := zones[zoneIds[i]]; ok {
			rows[i].Zone = zones[zoneIds[i]].Name
			rows[i].ZoneExtId = fetchExternalId(zones[zoneIds[i]].ExternalId)
			rows[i].CloudregionId = zones[zoneIds[i]].CloudregionId
		}
		regions[i] = &SCloudregionResourceBase{rows[i].CloudregionId}
	}

	regionRows := manager.SCloudregionResourceBaseManager.FetchCustomizeColumns(ctx, userCred, query, regions, fields, isList)
	for i := range rows {
		rows[i].CloudregionResourceInfo = regionRows[i]
	}

	return rows
}

func (manager *SZoneResourceBaseManager) ListItemFilter(
	ctx context.Context,
	q *sqlchemy.SQuery,
	userCred mcclient.TokenCredential,
	query api.ZonalFilterListInput,
) (*sqlchemy.SQuery, error) {
	q, err := managedResourceFilterByZone(q, query, "", nil)
	if err != nil {
		return nil, errors.Wrap(err, "managedResourceFilterByZone")
	}
	subq := ZoneManager.Query("id").Snapshot()
	subq, err = manager.SCloudregionResourceBaseManager.ListItemFilter(ctx, subq, userCred, query.RegionalFilterListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SCloudregionResourceBaseManager.ListItemFilter")
	}
	if subq.IsAltered() {
		q = q.Filter(sqlchemy.In(q.Field("zone_id"), subq.SubQuery()))
	}
	return q, nil
}

func (manager *SZoneResourceBaseManager) QueryDistinctExtraField(q *sqlchemy.SQuery, field string) (*sqlchemy.SQuery, error) {
	switch field {
	case "zone":
		zoneQuery := ZoneManager.Query("name", "id").Distinct().SubQuery()
		q.AppendField(zoneQuery.Field("name", field))
		q = q.Join(zoneQuery, sqlchemy.Equals(q.Field("zone_id"), zoneQuery.Field("id")))
		q = q.GroupBy(zoneQuery.Field("name"))
		return q, nil
	}
	zones := ZoneManager.Query("id", "cloudregion_id").SubQuery()
	q = q.LeftJoin(zones, sqlchemy.Equals(q.Field("zone_id"), zones.Field("id")))
	q, err := manager.SCloudregionResourceBaseManager.QueryDistinctExtraField(q, field)
	if err == nil {
		return q, nil
	}
	return q, httperrors.ErrNotFound
}

func (manager *SZoneResourceBaseManager) OrderByExtraFields(
	ctx context.Context,
	q *sqlchemy.SQuery,
	userCred mcclient.TokenCredential,
	query api.ZonalFilterListInput,
) (*sqlchemy.SQuery, error) {
	q, orders, fields := manager.GetOrderBySubQuery(q, userCred, query)
	if len(orders) > 0 {
		q = db.OrderByFields(q, orders, fields)
	}
	return q, nil
}

func (manager *SZoneResourceBaseManager) GetOrderBySubQuery(
	q *sqlchemy.SQuery,
	userCred mcclient.TokenCredential,
	query api.ZonalFilterListInput,
) (*sqlchemy.SQuery, []string, []sqlchemy.IQueryField) {
	zoneQ := ZoneManager.Query("id", "name")
	var orders []string
	var fields []sqlchemy.IQueryField
	if db.NeedOrderQuery(manager.SCloudregionResourceBaseManager.GetOrderByFields(query.RegionalFilterListInput)) {
		zoneQ, orders, fields = manager.SCloudregionResourceBaseManager.GetOrderBySubQuery(zoneQ, userCred, query.RegionalFilterListInput)
	}
	if db.NeedOrderQuery(manager.GetOrderByFields(query)) {
		subq := zoneQ.SubQuery()
		q = q.LeftJoin(subq, sqlchemy.Equals(q.Field("zone_id"), subq.Field("id")))
		if db.NeedOrderQuery([]string{query.OrderByZone}) {
			orders = append(orders, query.OrderByZone)
			fields = append(fields, subq.Field("name"))
		}
	}
	return q, orders, fields
}

func (manager *SZoneResourceBaseManager) GetOrderByFields(query api.ZonalFilterListInput) []string {
	orders := make([]string, 0)
	zoneOrders := manager.SCloudregionResourceBaseManager.GetOrderByFields(query.RegionalFilterListInput)
	orders = append(orders, zoneOrders...)
	orders = append(orders, query.OrderByZone)
	return orders
}
