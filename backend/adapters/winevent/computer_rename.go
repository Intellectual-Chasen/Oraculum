package winevent

// ComputerRenameOf は、端末の名前の変更を記録した Event (プロバイダ EventLog の 6011) から、変更の
// 前と後の名前を返す。ok が偽になるのは、Event が名前の変更の記録でないとき、または名前の無い
// `<Data>` の値を 2 つ以上持たないときである。
//
// 6011 は変更の前の名前と後の名前を、名前の無い `<Data>` にこの順で持つ。
func ComputerRenameOf(event Event) (previous, current string, ok bool) {
	system := event.System
	if system.ProviderName == nil || *system.ProviderName != "EventLog" ||
		system.EventID == nil || *system.EventID != "6011" {
		return "", "", false
	}
	var names []string
	for _, value := range event.EventData {
		if value.Name == "" {
			names = append(names, value.Text)
		}
	}
	if len(names) < 2 || names[0] == "" || names[1] == "" {
		return "", "", false
	}
	return names[0], names[1], true
}
