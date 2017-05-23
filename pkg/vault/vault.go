// Copyright © 2017 Jetstack Ltd. <james@jetstack.io>
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

package vault

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/hashicorp/vault/api"
	"gitlab.jetstack.net/jetstack-experimental/vault-unsealer/pkg/kv"
)

// vault is an implementation of the Vault interface that will perform actions
// against a Vault server, using a provided KMS to retreive
type vault struct {
	keyStore kv.Service
	cl       *api.Client
	prefix   string
}

var _ Vault = &vault{}

// Vault is an interface that can be used to attempt to perform actions against
// a Vault server.
type Vault interface {
	Sealed() (bool, error)
	Unseal() error
	Init() error
}

// New returns a new vault Vault, or an error.
func New(prefix string, k kv.Service, cl *api.Client) (Vault, error) {
	return &vault{k, cl, prefix}, nil
}

func (u *vault) Sealed() (bool, error) {
	resp, err := u.cl.Sys().SealStatus()
	if err != nil {
		return false, fmt.Errorf("error checking status: %s", err.Error())
	}
	return resp.Sealed, nil
}

// Unseal will attempt to unseal vault by retrieving keys from the kms service
// and sending unseal requests to vault. It will return an error if retrieving
// a key fails, or if the unseal progress is reset to 0 (indicating that a key)
// was invalid.
func (u *vault) Unseal() error {
	for i := 0; ; i++ {
		keyID := u.unsealKeyForID(i)

		logrus.Debugf("retrieving key from kms service...")
		k, err := u.keyStore.Get(keyID)

		if err != nil {
			return fmt.Errorf("unable to get key '%s': %s", keyID, err.Error())
		}

		logrus.Debugf("sending unseal request to vault...")
		resp, err := u.cl.Sys().Unseal(string(k))

		if err != nil {
			return fmt.Errorf("fail to send unseal request to vault: %s", err.Error())
		}

		logrus.Debugf("got unseal response: %+v", *resp)

		if !resp.Sealed {
			return nil
		}

		// if progress is 0, we failed to unseal vault.
		if resp.Progress == 0 {
			return fmt.Errorf("failed to unseal vault. progress reset to 0")
		}
	}
}

func (u *vault) Init() error {
	resp, err := u.cl.Sys().Init(&api.InitRequest{
		SecretShares:    5,
		SecretThreshold: 3,
	})

	if err != nil {
		return fmt.Errorf("error initialising vault: %s", err.Error())
	}

	for i, k := range resp.Keys {
		keyID := u.unsealKeyForID(i)
		err := u.keyStore.Set(keyID, k)

		if err != nil {
			return fmt.Errorf("error storing unseal key: %s", keyID)
		}
	}

	rootTokenKey := u.rootTokenKey()

	if err = u.keyStore.Set(rootTokenKey, resp.RootToken); err != nil {
		return fmt.Errorf("error storing root token: %s", rootTokenKey)
	}

	return nil
}

func (u *vault) unsealKeyForID(i int) string {
	return fmt.Sprintf("%s-unseal-%d", u.prefix, i)
}

func (u *vault) rootTokenKey() string {
	return fmt.Sprintf("%s-root", u.prefix)
}