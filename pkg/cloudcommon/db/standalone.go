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

package db

import (
	"context"
	"fmt"
	"strings"

	"yunion.io/x/jsonutils"
	"yunion.io/x/pkg/errors"
	"yunion.io/x/pkg/util/regutils"
	"yunion.io/x/pkg/util/stringutils"
	"yunion.io/x/pkg/utils"
	"yunion.io/x/sqlchemy"

	"yunion.io/x/onecloud/pkg/apis"
	"yunion.io/x/onecloud/pkg/cloudcommon/consts"
	"yunion.io/x/onecloud/pkg/cloudcommon/policy"
	"yunion.io/x/onecloud/pkg/httperrors"
	"yunion.io/x/onecloud/pkg/mcclient"
	"yunion.io/x/onecloud/pkg/util/rbacutils"
)

type UUIDGenerator func() string

var (
	DefaultUUIDGenerator = stringutils.UUID4
)

type SStandaloneResourceBase struct {
	SResourceBase

	// 资源UUID
	Id string `width:"128" charset:"ascii" primary:"true" list:"user" json:"id"`
	// 资源名称
	Name string `width:"128" charset:"utf8" nullable:"false" index:"true" list:"user" update:"user" create:"required" json:"name"`

	// 资源描述信息
	Description string `width:"256" charset:"utf8" get:"user" list:"user" update:"user" create:"optional" json:"description"`

	// 是否是模拟资源, 部分从公有云上同步的资源并不真实存在, 例如宿主机
	// list 接口默认不会返回这类资源，除非显示指定 is_emulate=true 过滤参数
	IsEmulated bool `nullable:"false" default:"false" list:"admin" create:"admin_optional" json:"is_emulated"`
}

func (model *SStandaloneResourceBase) BeforeInsert() {
	if len(model.Id) == 0 {
		model.Id = DefaultUUIDGenerator()
	}
}

type SStandaloneResourceBaseManager struct {
	SResourceBaseManager
	NameRequireAscii bool
	NameLength       int
}

func NewStandaloneResourceBaseManager(dt interface{}, tableName string, keyword string, keywordPlural string) SStandaloneResourceBaseManager {
	return SStandaloneResourceBaseManager{SResourceBaseManager: NewResourceBaseManager(dt, tableName, keyword, keywordPlural)}
}

func (manager *SStandaloneResourceBaseManager) IsStandaloneManager() bool {
	return true
}

func (manager *SStandaloneResourceBaseManager) GetIStandaloneModelManager() IStandaloneModelManager {
	return manager.GetVirtualObject().(IStandaloneModelManager)
}

func (manager *SStandaloneResourceBaseManager) FilterById(q *sqlchemy.SQuery, idStr string) *sqlchemy.SQuery {
	return q.Equals("id", idStr)
}

func (manager *SStandaloneResourceBaseManager) FilterByNotId(q *sqlchemy.SQuery, idStr string) *sqlchemy.SQuery {
	return q.NotEquals("id", idStr)
}

func (manager *SStandaloneResourceBaseManager) FilterByName(q *sqlchemy.SQuery, name string) *sqlchemy.SQuery {
	return q.Equals("name", name)
}

func (manager *SStandaloneResourceBaseManager) FilterByHiddenSystemAttributes(q *sqlchemy.SQuery, userCred mcclient.TokenCredential, query jsonutils.JSONObject, scope rbacutils.TRbacScope) *sqlchemy.SQuery {
	q = manager.SResourceBaseManager.FilterByHiddenSystemAttributes(q, userCred, query, scope)
	showEmulated := jsonutils.QueryBoolean(query, "show_emulated", false)
	if showEmulated {
		var isAllow bool
		if consts.IsRbacEnabled() {
			allowScope := policy.PolicyManager.AllowScope(userCred, consts.GetServiceType(), manager.KeywordPlural(), policy.PolicyActionList, "show_emulated")
			if !scope.HigherThan(allowScope) {
				isAllow = true
			}
		} else {
			if userCred.HasSystemAdminPrivilege() {
				isAllow = true
			}
		}
		if !isAllow {
			showEmulated = false
		}
	}
	if !showEmulated {
		q = q.IsFalse("is_emulated")
	}
	return q
}

func (manager *SStandaloneResourceBaseManager) ValidateName(name string) error {
	if manager.NameRequireAscii && !regutils.MatchName(name) {
		return httperrors.NewInputParameterError("name starts with letter, and contains letter, number and ._@- only")
	}
	if manager.NameLength > 0 && len(name) > manager.NameLength {
		return httperrors.NewInputParameterError("name longer than %d", manager.NameLength)
	}
	return nil
}

func (manager *SStandaloneResourceBaseManager) FetchById(idStr string) (IModel, error) {
	return FetchById(manager.GetIStandaloneModelManager(), idStr)
}

func (manager *SStandaloneResourceBaseManager) FetchByName(userCred mcclient.IIdentityProvider, idStr string) (IModel, error) {
	return FetchByName(manager.GetIStandaloneModelManager(), userCred, idStr)
}

func (manager *SStandaloneResourceBaseManager) FetchByIdOrName(userCred mcclient.IIdentityProvider, idStr string) (IModel, error) {
	return FetchByIdOrName(manager.GetIStandaloneModelManager(), userCred, idStr)
}

func (manager *SStandaloneResourceBaseManager) ListItemFilter(ctx context.Context, q *sqlchemy.SQuery, userCred mcclient.TokenCredential, input apis.StandaloneResourceListInput) (*sqlchemy.SQuery, error) {
	q, err := manager.SResourceBaseManager.ListItemFilter(ctx, q, userCred, input.ResourceBaseListInput)
	if err != nil {
		return q, errors.Wrap(err, "SResourceBaseManager.ListItemFilte")
	}

	if len(input.Names) > 0 {
		q = q.In("name", input.Names)
	}

	if len(input.Ids) > 0 {
		q = q.In("id", input.Ids)
	}

	tags := map[string][]string{}
	for _, tag := range input.Tags {
		if _, ok := tags[tag.Key]; !ok {
			tags[tag.Key] = []string{}
		}
		if len(tag.Value) > 0 && !utils.IsInStringArray(tag.Value, tags[tag.Key]) {
			tags[tag.Key] = append(tags[tag.Key], tag.Value)
		}
	}

	if len(tags) > 0 {
		metadataView := Metadata.Query()
		idx := 0
		for key, values := range tags {
			if idx == 0 {
				metadataView = metadataView.Equals("key", key)
				if len(values) > 0 {
					metadataView = metadataView.In("value", values)
				}
			} else {
				subMetataView := Metadata.Query().Equals("key", key)
				if len(values) > 0 {
					subMetataView = subMetataView.In("value", values)
				}
				sq := subMetataView.SubQuery()
				metadataView.Join(sq, sqlchemy.Equals(metadataView.Field("id"), sq.Field("id")))
			}
			idx++
		}
		metadatas := metadataView.SubQuery()
		fieldName := fmt.Sprintf("%s_id", manager.Keyword())
		metadataSQ := metadatas.Query(
			sqlchemy.REPLACE(fieldName, metadatas.Field("id"), manager.Keyword()+"::", ""),
		)
		sq := metadataSQ.Filter(sqlchemy.Like(metadatas.Field("id"), manager.Keyword()+"::%")).Distinct()
		q = q.Filter(sqlchemy.In(q.Field("id"), sq))
	}

	if input.WithoutUserMeta {
		metadatas := Metadata.Query().SubQuery()
		fieldName := fmt.Sprintf("%s_id", manager.Keyword())
		metadataSQ := metadatas.Query(
			sqlchemy.REPLACE(fieldName, metadatas.Field("id"), manager.Keyword()+"::", ""),
		)
		sq := metadataSQ.Filter(sqlchemy.Like(metadatas.Field("key"), USER_TAG_PREFIX+"%")).Distinct()

		q.Filter(sqlchemy.NotIn(q.Field("id"), sq))
	}

	return q, nil
}

func (model *SStandaloneResourceBase) StandaloneModelManager() IStandaloneModelManager {
	return model.GetModelManager().(IStandaloneModelManager)
}

func (model *SStandaloneResourceBase) GetId() string {
	return model.Id
}

func (model *SStandaloneResourceBase) GetName() string {
	return model.Name
}

func (model *SStandaloneResourceBase) GetIStandaloneModel() IStandaloneModel {
	return model.GetVirtualObject().(IStandaloneModel)
}

func (model *SStandaloneResourceBase) GetShortDesc(ctx context.Context) *jsonutils.JSONDict {
	desc := model.SResourceBase.GetShortDesc(ctx)
	desc.Add(jsonutils.NewString(model.GetName()), "name")
	desc.Add(jsonutils.NewString(model.GetId()), "id")
	/*if len(model.ExternalId) > 0 {
		desc.Add(jsonutils.NewString(model.ExternalId), "external_id")
	}*/
	return desc
}

func (model *SStandaloneResourceBase) GetShortDescV2(ctx context.Context) *apis.StandaloneResourceShortDescDetail {
	desc := &apis.StandaloneResourceShortDescDetail{}
	desc.ModelBaseShortDescDetail = *model.SResourceBase.GetShortDescV2(ctx)
	desc.Name = model.GetName()
	desc.Id = model.GetId()
	return desc
}

func (model *SStandaloneResourceBase) GetExtraDetails(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, isList bool) (apis.StandaloneResourceDetails, error) {
	var err error
	out := apis.StandaloneResourceDetails{}
	out.ModelBaseDetails, err = model.SResourceBase.GetExtraDetails(ctx, userCred, query, isList)
	if err != nil {
		return out, err
	}
	return out, nil
}

/*
 * userCred: optional
 */
func (model *SStandaloneResourceBase) GetMetadata(key string, userCred mcclient.TokenCredential) string {
	return Metadata.GetStringValue(model, key, userCred)
}

func (model *SStandaloneResourceBase) GetMetadataJson(key string, userCred mcclient.TokenCredential) jsonutils.JSONObject {
	return Metadata.GetJsonValue(model, key, userCred)
}

func (model *SStandaloneResourceBase) SetMetadata(ctx context.Context, key string, value interface{}, userCred mcclient.TokenCredential) error {
	return Metadata.SetValue(ctx, model, key, value, userCred)
}

func (model *SStandaloneResourceBase) SetAllMetadata(ctx context.Context, dictstore map[string]interface{}, userCred mcclient.TokenCredential) error {
	return Metadata.SetValuesWithLog(ctx, model, dictstore, userCred)
}

func (model *SStandaloneResourceBase) SetUserMetadataValues(ctx context.Context, dictstore map[string]interface{}, userCred mcclient.TokenCredential) error {
	return Metadata.SetValuesWithLog(ctx, model, dictstore, userCred)
}

func (model *SStandaloneResourceBase) SetUserMetadataAll(ctx context.Context, dictstore map[string]interface{}, userCred mcclient.TokenCredential) error {
	return Metadata.SetAll(ctx, model, dictstore, userCred, USER_TAG_PREFIX)
}

func (model *SStandaloneResourceBase) SetCloudMetadataAll(ctx context.Context, dictstore map[string]interface{}, userCred mcclient.TokenCredential) error {
	return Metadata.SetAll(ctx, model, dictstore, userCred, CLOUD_TAG_PREFIX)
}

func (model *SStandaloneResourceBase) RemoveMetadata(ctx context.Context, key string, userCred mcclient.TokenCredential) error {
	return Metadata.SetValue(ctx, model, key, "", userCred)
}

func (model *SStandaloneResourceBase) RemoveAllMetadata(ctx context.Context, userCred mcclient.TokenCredential) error {
	return Metadata.RemoveAll(ctx, model, userCred)
}

func (model *SStandaloneResourceBase) GetAllMetadata(userCred mcclient.TokenCredential) (map[string]string, error) {
	return Metadata.GetAll(model, nil, userCred)
}

func (model *SStandaloneResourceBase) AllowGetDetailsMetadata(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject) bool {
	return IsAllowGetSpec(rbacutils.ScopeSystem, userCred, model, "metadata")
}

func (model *SStandaloneResourceBase) GetDetailsMetadata(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject) (jsonutils.JSONObject, error) {
	fields := jsonutils.GetQueryStringArray(query, "field")
	val, err := Metadata.GetAll(model, fields, userCred)
	if err != nil {
		return nil, err
	}
	return jsonutils.Marshal(val), nil
}

func (model *SStandaloneResourceBase) AllowPerformMetadata(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) bool {
	return IsAllowPerform(rbacutils.ScopeSystem, userCred, model, "metadata")
}

func (model *SStandaloneResourceBase) PerformMetadata(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) (jsonutils.JSONObject, error) {
	dict, ok := data.(*jsonutils.JSONDict)
	if !ok {
		return nil, httperrors.NewInputParameterError("input data not key value dict")
	}
	dictMap, err := dict.GetMap()
	if err != nil {
		return nil, err
	}
	dictStore := make(map[string]interface{})
	for k, v := range dictMap {
		// 已双下滑线开头的metadata是系统内置，普通用户不可添加，只能查看
		if strings.HasPrefix(k, SYS_TAG_PREFIX) && (userCred == nil || !IsAllowPerform(rbacutils.ScopeSystem, userCred, model, "metadata")) {
			return nil, httperrors.NewForbiddenError("not allow to set system key, please remove the underscore at the beginning")
		}
		dictStore[k], _ = v.GetString()
	}
	err = model.SetAllMetadata(ctx, dictStore, userCred)
	return nil, err
}

func (model *SStandaloneResourceBase) AllowPerformUserMetadata(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) bool {
	return IsAllowPerform(rbacutils.ScopeSystem, userCred, model, "user-metadata")
}

func (model *SStandaloneResourceBase) PerformUserMetadata(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) (jsonutils.JSONObject, error) {
	dict, ok := data.(*jsonutils.JSONDict)
	if !ok {
		return nil, httperrors.NewInputParameterError("input data not key value dict")
	}
	dictMap, err := dict.GetMap()
	if err != nil {
		return nil, err
	}
	dictStore := make(map[string]interface{})
	for k, v := range dictMap {
		dictStore[USER_TAG_PREFIX+k], _ = v.GetString()
	}
	err = model.SetUserMetadataValues(ctx, dictStore, userCred)
	return nil, err
}

func (model *SStandaloneResourceBase) AllowPerformSetUserMetadata(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) bool {
	return IsAllowPerform(rbacutils.ScopeSystem, userCred, model, "set-user-metadata")
}

func (model *SStandaloneResourceBase) PerformSetUserMetadata(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) (jsonutils.JSONObject, error) {
	dict, ok := data.(*jsonutils.JSONDict)
	if !ok {
		return nil, httperrors.NewInputParameterError("input data not key value dict")
	}
	dictMap, err := dict.GetMap()
	if err != nil {
		return nil, err
	}
	dictStore := make(map[string]interface{})
	for k, v := range dictMap {
		dictStore[USER_TAG_PREFIX+k], _ = v.GetString()
	}
	err = model.SetUserMetadataAll(ctx, dictStore, userCred)
	return nil, err
}

func (model *SStandaloneResourceBase) PostUpdate(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) {
	model.SResourceBase.PostUpdate(ctx, userCred, query, data)

	jsonMeta, _ := data.Get("__meta__")
	if jsonMeta != nil {
		model.PerformMetadata(ctx, userCred, nil, jsonMeta)
	}
}

func (model *SStandaloneResourceBase) PostCreate(ctx context.Context, userCred mcclient.TokenCredential, ownerId mcclient.IIdentityProvider, query jsonutils.JSONObject, data jsonutils.JSONObject) {
	model.SResourceBase.PostCreate(ctx, userCred, ownerId, query, data)

	jsonMeta, _ := data.Get("__meta__")
	if jsonMeta != nil {
		model.PerformMetadata(ctx, userCred, nil, jsonMeta)
	}
}

func (model *SStandaloneResourceBase) PostDelete(ctx context.Context, userCred mcclient.TokenCredential) {
	if model.Deleted {
		model.RemoveAllMetadata(ctx, userCred)
	}
	model.SResourceBase.PostDelete(ctx, userCred)
}

// func (model *SStandaloneResourceBase) Delete(ctx context.Context, userCred mcclient.TokenCredential) error {
// 	return DeleteModel(ctx, userCred, model)
// }

func (model *SStandaloneResourceBase) ClearSchedDescCache() error {
	return nil
}

func (model *SStandaloneResourceBase) AppendDescription(userCred mcclient.TokenCredential, msg string) error {
	_, err := Update(model.GetIStandaloneModel(), func() error {
		if len(model.Description) > 0 {
			model.Description += ";"
		}
		model.Description += msg
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "db.Update")
	}
	OpsLog.LogEvent(model, "append_desc", msg, userCred)
	return nil
}

func (manager *SStandaloneResourceBaseManager) ValidateCreateData(ctx context.Context, userCred mcclient.TokenCredential, ownerId mcclient.IIdentityProvider, query jsonutils.JSONObject, input apis.StandaloneResourceCreateInput) (apis.StandaloneResourceCreateInput, error) {
	var err error
	input.ResourceBaseCreateInput, err = manager.SResourceBaseManager.ValidateCreateData(ctx, userCred, ownerId, query, input.ResourceBaseCreateInput)
	if err != nil {
		return input, errors.Wrap(err, "SResourceBaseManager.ValidateCreateData")
	}
	return input, nil
}

/*
func (model SStandaloneResourceBase) GetExternalId() string {
	return model.ExternalId
}

func (model *SStandaloneResourceBase) SetExternalId(userCred mcclient.TokenCredential, idstr string) error {
	if model.ExternalId != idstr {
		diff, err := Update(model, func() error {
			model.ExternalId = idstr
			return nil
		})
		if err == nil {
			OpsLog.LogEvent(model, ACT_UPDATE, diff, userCred)
		}
		return err
	}
	return nil
}
*/
