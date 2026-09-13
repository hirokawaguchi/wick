package reason

// 切断理由。原典 LOGDAT.H に合わせる。
// Online(0) は Wick 追加。接続時に先行して記録する行の「まだ切断していない」印。
const (
	Online       = 0
	CarrierDown  = 1
	TimeOut      = 2
	CannotLogin  = 3
	LogOff       = 4
	Duplicate    = 5
	ChannelError = 6
	SystemError  = 7
	Absolute     = 8
)

func Name(r int) string {
	switch r {
	case Online:
		return "online"
	case CarrierDown:
		return "carrier_down"
	case TimeOut:
		return "time_out"
	case CannotLogin:
		return "cannot_login"
	case LogOff:
		return "log_off"
	case Duplicate:
		return "duplicate"
	case ChannelError:
		return "channel_error"
	case SystemError:
		return "system_error"
	case Absolute:
		return "absolute"
	default:
		return "unknown"
	}
}
