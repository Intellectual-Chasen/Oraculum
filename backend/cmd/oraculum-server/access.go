package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// adminFlag は、調査の管理者にするアカウントのログイン名を渡す flag である。
//
// 起動のたびに、そのアカウントを管理者にする。最初の管理者を登録し、管理者を失った調査を
// 運用者が取り戻す手段である。
const adminFlag = "--admin"

// prepareAccess は、調査の役割の保存先を返す。admin を渡した起動は、そのアカウントを管理者にする。
//
// **役割を持つ利用者がいない起動を止める。** 誰も調査を開けず、役割を与えることもできない。
// 調査の directory を渡さない起動の役割はメモリに置き、停止すると消える。
func prepareAccess(
	ctx context.Context, admin string, accounts api.Accounts, investigation *pipeline.Investigation,
) (*pipeline.AccessStore, error) {
	access := pipeline.NewAccessStore(nil)
	if investigation != nil {
		access = investigation.Access()
	}
	if admin != "" {
		if _, err := accounts.Account(ctx, admin); err != nil {
			if errors.Is(err, api.ErrAccountNotFound) {
				return nil, fmt.Errorf("%s %q: no account has the login", adminFlag, admin)
			}
			return nil, fmt.Errorf("%s %q: %w", adminFlag, admin, err)
		}
		if role, ok := access.Role(admin); !ok || role != pipeline.RoleAdmin {
			if _, err := access.Grant(pipeline.OperatorActor, admin, pipeline.RoleAdmin); err != nil {
				return nil, fmt.Errorf("%s %q: %w", adminFlag, admin, err)
			}
		}
	}
	if len(access.List()) == 0 {
		return nil, fmt.Errorf("the investigation has no member; pass %s <login> to make an account its admin", adminFlag)
	}
	return access, nil
}
