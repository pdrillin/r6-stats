package rec

// mapNames maps the header "worldid" to a map name. Each entry was read from a
// replay whose map is known; unknown ids leave Header.Map empty.
var mapNames = map[string]string{
	"398899676157": "Fortress",
	"407558616688": "Chalet",
	"407987100456": "Border",
	"413779563590": "Bank",
	"413845419788": "Kafe Dostoyevsky",
	"417890697769": "Lair",
	"418119057546": "Nighthaven Labs",
	// Y11S3, recognised by their bomb sites.
	"407193663917": "Clubhouse",
	"409325881472": "Villa",
	"418126004176": "Consulate",
	"434715462383": "Oregon",
	"441408792952": "Yacht",
	"441825219764": "Outback",
	"447445273533": "Hereford Base",
	"454245490351": "Tower",
}
