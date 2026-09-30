// in-package test: 語彙の全項目を走査するため、非公開の対応表を直接読む。公開 API は
// 値を 1 つずつ問い合わせる形しか持たず、表に増えた項目を外から辿れない。
package core

import "testing"

// SemanticVocabularyKeys は対応表の項目を core_test 側の test から辿る窓口である。
//
// 宣言が _test.go の中だけにあるため、製品の API に出ない。期待の表と対応表を
// 両方向で突き合わせるために使う。
func SemanticVocabularyKeys() []SemanticKey {
	keys := make([]SemanticKey, 0, len(semanticVocabulary))
	for key := range semanticVocabulary {
		keys = append(keys, key)
	}
	return keys
}

// 対応表の各項目が既知の対象と役割を持つことを確かめる。
func TestSemanticVocabularyHoldsTheItemsOfTheTable(t *testing.T) {
	if len(semanticVocabulary) == 0 {
		t.Fatal("the vocabulary holds no item")
	}
	objects := map[SemanticObject]bool{
		SemanticObjectTerminal: true, SemanticObjectProcess: true,
		SemanticObjectFile: true, SemanticObjectRegistryValue: true,
		SemanticObjectAccount: true, SemanticObjectIp: true,
		SemanticObjectDomain: true, SemanticObjectConnection: true,
		SemanticObjectEvent: true, SemanticObjectRecord: true,
		SemanticObjectHttp: true,
	}
	roles := map[SemanticRole]bool{
		SemanticRoleIdentity: true, SemanticRoleLabel: true, SemanticRoleAttribute: true,
	}
	for key, meaning := range semanticVocabulary {
		if !objects[meaning.object] {
			t.Errorf("%q has the object %q, which the table does not name", key, meaning.object)
		}
		if !roles[meaning.role] {
			t.Errorf("%q has the role %q, which the table does not name", key, meaning.role)
		}
	}
}
