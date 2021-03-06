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

package shell

import (
	"yunion.io/x/jsonutils"

	"yunion.io/x/onecloud/pkg/mcclient"
	"yunion.io/x/onecloud/pkg/mcclient/modules"
	"yunion.io/x/onecloud/pkg/mcclient/options"
)

func init() {
	type DomainListOptions struct {
		options.BaseListOptions
	}
	R(&DomainListOptions{}, "domain-list", "List domains", func(s *mcclient.ClientSession, args *DomainListOptions) error {
		params, err := options.ListStructToParams(args)
		if err != nil {
			return err
		}
		result, err := modules.Domains.List(s, params)
		if err != nil {
			return err
		}
		printList(result, modules.Domains.GetColumns(s))
		return nil
	})

	type DomainDetailOptions struct {
		ID string `help:"ID or domain"`
	}
	R(&DomainDetailOptions{}, "domain-show", "Show detail of domain", func(s *mcclient.ClientSession, args *DomainDetailOptions) error {
		result, err := modules.Domains.Get(s, args.ID, nil)
		if err != nil {
			return err
		}
		printObject(result)
		return nil
	})
	R(&DomainDetailOptions{}, "domain-delete", "Delete a domain", func(s *mcclient.ClientSession, args *DomainDetailOptions) error {
		objId, err := modules.Domains.GetId(s, args.ID, nil)
		if err != nil {
			return err
		}
		result, err := modules.Domains.Delete(s, objId, nil)
		if err != nil {
			return err
		}
		printObject(result)
		return nil
	})

	/* R(&DomainDetailOptions{}, "domain-config-sql", "Config a domain with SQL driver", func(s *mcclient.ClientSession, args *DomainDetailOptions) error {
	    config := jsonutils.NewDict()
	    config.Add(jsonutils.NewString("sql"), "config", "identity", "driver")
	    objId, err := modules.Domains.GetId(s, args.ID, nil)
	    if err != nil {
	        return err
	    }
	    nconf, err := modules.Domains.UpdateConfig(s, objId, config)
	    if err != nil {
	        return err
	    }
	    fmt.Println(nconf.PrettyString())
	    return nil
	}) */

	type DomainCreateOptions struct {
		NAME     string `help:"Name of domain"`
		Desc     string `help:"Description"`
		Enabled  bool   `help:"Set the domain enabled"`
		Disabled bool   `help:"Set the domain disabled"`

		Displayname string `help:"display name"`
	}
	R(&DomainCreateOptions{}, "domain-create", "Create a new domain", func(s *mcclient.ClientSession, args *DomainCreateOptions) error {
		params := jsonutils.NewDict()
		params.Add(jsonutils.NewString(args.NAME), "name")
		if len(args.Desc) > 0 {
			params.Add(jsonutils.NewString(args.Desc), "description")
		}
		if args.Enabled && !args.Disabled {
			params.Add(jsonutils.JSONTrue, "enabled")
		} else if !args.Enabled && args.Disabled {
			params.Add(jsonutils.JSONFalse, "enabled")
		}
		if len(args.Displayname) > 0 {
			params.Add(jsonutils.NewString(args.Displayname), "displayname")
		}
		result, err := modules.Domains.Create(s, params)
		if err != nil {
			return err
		}
		printObject(result)
		return nil
	})

	type DomainUpdateOptions struct {
		ID       string `help:"ID of domain to update"`
		Name     string `help:"Name of domain"`
		Desc     string `help:"Description"`
		Enabled  bool   `help:"Set the domain enabled"`
		Disabled bool   `help:"Set the domain disabled"`
		Driver   string `help:"Set the domain Driver"`

		Displayname string `help:"display name"`
	}
	R(&DomainUpdateOptions{}, "domain-update", "Update a domain", func(s *mcclient.ClientSession, args *DomainUpdateOptions) error {
		obj, err := modules.Domains.Get(s, args.ID, nil)
		if err != nil {
			return err
		}
		objId, err := obj.GetString("id")
		if err != nil {
			return err
		}
		params := jsonutils.NewDict()
		if len(args.Name) > 0 {
			params.Add(jsonutils.NewString(args.Name), "name")
		}
		if len(args.Desc) > 0 {
			params.Add(jsonutils.NewString(args.Desc), "description")
		}
		if len(args.Driver) > 0 {
			params.Add(jsonutils.NewString(args.Driver), "driver")
		}

		if args.Enabled && !args.Disabled {
			params.Add(jsonutils.JSONTrue, "enabled")
		} else if !args.Enabled && args.Disabled {
			params.Add(jsonutils.JSONFalse, "enabled")
		}
		if len(args.Displayname) > 0 {
			params.Add(jsonutils.NewString(args.Displayname), "displayname")
		}
		result, err := modules.Domains.Patch(s, objId, params)
		if err != nil {
			return err
		}
		printObject(result)
		return nil
	})

}
