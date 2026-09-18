package dashboard

import "github.com/nunchimangchi-dev/unbrokerrdd/internal/db"

// Broker represents a single data broker target.
type Broker struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Strategy int    `json:"strategy"`
	URL      string `json:"url"`
	Status   Status `json:"status"`
}

// AllBrokers returns all 95 brokers as db.Broker slice for SQLite seeding.
// This is the single source of truth shared between the dashboard and the DB.
func AllBrokers() []db.Broker {
	raw := initBrokers()
	out := make([]db.Broker, len(raw))
	for i, b := range raw {
		out[i] = db.Broker{
			ID:       b.ID,
			Name:     b.Name,
			Strategy: b.Strategy,
			URL:      b.URL,
			Status:   db.StatusPending,
		}
	}
	return out
}

// Status represents the current state of a broker opt-out.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusSuccess    Status = "success"
	StatusFailed     Status = "failed"
	StatusSkipped    Status = "skipped"
	StatusManual     Status = "manual"
)

// initBrokers returns all 95 data brokers with their strategies, all starting pending.
func initBrokers() []Broker {
	return []Broker{
		// ── Strategy 1: TruthFinder Affiliates (7 sites, 1 submission) ──────────
		{ID: "backgroundcheckme", Name: "Backgroundcheckme.org", Strategy: 1, URL: "backgroundcheckme.org"},
		{ID: "newyorkpublicrecords", Name: "NewYorkPublicRecords.org", Strategy: 1, URL: "newyorkpublicrecords.org"},
		{ID: "oregonpublicrecords", Name: "OregonPublicRecords.org", Strategy: 1, URL: "oregonpublicrecords.org"},
		{ID: "publicrecordscenter", Name: "PublicRecordsCenter", Strategy: 1, URL: "publicrecordscenter.org"},
		{ID: "publicrecordsreviews", Name: "PublicRecordsReviews", Strategy: 1, URL: "publicrecordsreviews.com"},
		{ID: "publicsrecords", Name: "PublicsRecords", Strategy: 1, URL: "publicsrecords.com"},
		{ID: "top4backgroundchecks", Name: "Top4Backgroundchecks", Strategy: 1, URL: "top4backgroundchecks.com"},

		// ── Strategy 2: Hidden Opt-Out Pages (14 sites) ──────────────────────────
		// "peeplookup" replaced 2026-09-17 with a verified real target (CheckPeople,
		// from the community-maintained BADBOOL opt-out list) — the only Strategy 2
		// site with a working automated handler so far; see strategies/strategy2_checkpeople.go.
		{ID: "checkpeople", Name: "CheckPeople", Strategy: 2, URL: "checkpeople.com"},
		// Added 2026-09-18 (not a replacement — bumps the true total to 86,
		// since none of the existing 14 entries were confirmed as this site's
		// duplicate). Verified live; chromedp reaches the real form without
		// getting stuck on a bot-check, unlike several other BADBOOL sites
		// tried the same day. See strategies/strategy2_advancedbackgroundchecks.go.
		{ID: "advancedbackgroundchecks", Name: "AdvancedBackgroundChecks", Strategy: 2, URL: "advancedbackgroundchecks.com"},
		{ID: "ohioresidentdirectory", Name: "Ohio Resident Directory", Strategy: 2, URL: "ohioresidentdirectory.com"},
		{ID: "peoplesearchexpert", Name: "People Search Expert", Strategy: 2, URL: "peoplesearchexpert.com"},
		{ID: "peoplefastfind", Name: "PeopleFastFind", Strategy: 2, URL: "peoplefastfind.com"},
		{ID: "backgroundchecksorg", Name: "Background Checks.org", Strategy: 2, URL: "backgroundchecks.org"},
		{ID: "backgroundchecksme", Name: "BackgroundChecks.me", Strategy: 2, URL: "backgroundchecks.me"},
		{ID: "freepeoplesearch", Name: "FreePeopleSearch.com", Strategy: 2, URL: "freepeople-search.com"},
		{ID: "usawhitepages", Name: "USAWhitepages", Strategy: 2, URL: "usawhitepages.com"},
		{ID: "usaphonesbook", Name: "USAPhonesBook", Strategy: 2, URL: "usaphonesbook.com"},
		{ID: "locatefamily", Name: "LocateFamily.com", Strategy: 2, URL: "locatefamily.com"},
		{ID: "reuniondotcom", Name: "Reunion.com", Strategy: 2, URL: "reunion.com"},
		{ID: "addrhistory", Name: "AddrHistory", Strategy: 2, URL: "addrhistory.com"},
		{ID: "alumnius", Name: "Alumni US", Strategy: 2, URL: "alumnius.net"},
		{ID: "jailbase", Name: "JailBase", Strategy: 2, URL: "jailbase.com"},
		// Added 2026-09-18: investigated during the BADBOOL sweep (see
		// HANDOFF.md 2026-09-18) but never actually added as registry rows
		// until now - that real reconnaissance work wasn't reflected in the
		// tracked system. Domains verified via web search against the
		// BADBOOL list, not guessed (in particular "Clustal" is clustal.org,
		// not .com - easy to mistype). blocker_type set for each right
		// after seeding, matching the already-documented reasons.
		{ID: "spokeo", Name: "Spokeo", Strategy: 2, URL: "spokeo.com"},
		{ID: "beenverified", Name: "BeenVerified", Strategy: 2, URL: "beenverified.com"},
		{ID: "smartbackgroundchecks", Name: "SmartBackgroundChecks", Strategy: 2, URL: "smartbackgroundchecks.com"},
		{ID: "nuwber", Name: "Nuwber", Strategy: 2, URL: "nuwber.com"},
		{ID: "clustal", Name: "Clustal", Strategy: 2, URL: "clustal.org"},
		{ID: "thatsthem", Name: "That's Them", Strategy: 2, URL: "thatsthem.com"},
		{ID: "familytreenow", Name: "FamilyTreeNow", Strategy: 2, URL: "familytreenow.com"},
		{ID: "usphonebook", Name: "USPhoneBook", Strategy: 2, URL: "usphonebook.com"},
		{ID: "radaris", Name: "Radaris", Strategy: 2, URL: "radaris.com"},

		// ── Strategy 3: Privacy Page Discovery (25 sites) ────────────────────────
		{ID: "alignable", Name: "Alignable", Strategy: 3, URL: "alignable.com"},
		{ID: "allpeople", Name: "AllPeople.biz", Strategy: 3, URL: "allpeople.biz"},
		{ID: "enpnetwork", Name: "ENP Network", Strategy: 3, URL: "enpnetwork.com"},
		{ID: "konaequity", Name: "Kona Equity", Strategy: 3, URL: "konaequity.com"},
		{ID: "morningstar", Name: "Morningstar", Strategy: 3, URL: "morningstar.com"},
		{ID: "listmatch", Name: "ListMatch", Strategy: 3, URL: "listmatch.com"},
		{ID: "cityzor", Name: "Cityzor", Strategy: 3, URL: "cityzor.com"},
		{ID: "criminalpages", Name: "CriminalPages.com", Strategy: 3, URL: "criminalpages.com"},
		{ID: "genealogicreview", Name: "Genealogic.review", Strategy: 3, URL: "genealogic.review"},
		{ID: "lacountydearrest", Name: "LA County Arrest Records", Strategy: 3, URL: "lacountyarrestrecords.com"},
		{ID: "myfunnyprofile", Name: "My Funny Profile", Strategy: 3, URL: "myfunnyprofile.com"},
		{ID: "ndpeoplerecords", Name: "ND People Records", Strategy: 3, URL: "northdakotapeoplerecords.com"},
		{ID: "mapeoplerecords", Name: "MA People Records", Strategy: 3, URL: "massachusettspeoplerecords.com"},
		{ID: "oldfriends", Name: "Old-Friends.co", Strategy: 3, URL: "old-friends.co"},
		{ID: "oldphonebook", Name: "OldPhoneBook.com", Strategy: 3, URL: "oldphonebook.com"},
		{ID: "opendatausa", Name: "OpenDataUSA", Strategy: 3, URL: "opendatausa.com"},
		{ID: "stageresearch", Name: "StageResearch", Strategy: 3, URL: "stageresearch.com"},
		{ID: "realtyverify", Name: "RealtyVerify", Strategy: 3, URL: "realtyverify.com"},
		{ID: "parcellookup", Name: "ParcelLookup.com", Strategy: 3, URL: "parcellookup.com"},
		{ID: "censusinfous", Name: "census-info.us", Strategy: 3, URL: "census-info.us"},
		{ID: "knsee", Name: "knsee.com", Strategy: 3, URL: "knsee.com"},
		{ID: "pininthemap", Name: "pininthemap.com", Strategy: 3, URL: "pininthemap.com"},
		{ID: "pplchecker", Name: "pplChecker", Strategy: 3, URL: "pplchecker.com"},
		{ID: "h1bdata", Name: "h1bdata.info", Strategy: 3, URL: "h1bdata.info"},
		{ID: "imagemaps", Name: "image-maps.com", Strategy: 3, URL: "image-maps.com"},

		// ── Strategy 4: Business Directories (18 sites) ──────────────────────────
		{ID: "amfibi", Name: "Amfibi", Strategy: 4, URL: "amfibi.com"},
		{ID: "azcorpcorp", Name: "AZ Corp Commission", Strategy: 4, URL: "azcc.gov"},
		{ID: "ausibiz", Name: "AusiBiz", Strategy: 4, URL: "ausibiz.com"},
		{ID: "cabizdb", Name: "CA Business Database", Strategy: 4, URL: "businesssearch.sos.ca.gov"},
		{ID: "canadacompany", Name: "Canada Company Registry", Strategy: 4, URL: "corporationscanada.ic.gc.ca"},
		{ID: "georgiacompany", Name: "Georgia Company Registry", Strategy: 4, URL: "ecorp.sos.ga.gov"},
		{ID: "merchantcircle", Name: "MerchantCircle", Strategy: 4, URL: "merchantcircle.com"},
		{ID: "misterwhat", Name: "MisterWhat", Strategy: 4, URL: "misterwhat.com"},
		{ID: "opencorpdata", Name: "OpenCorpData", Strategy: 4, URL: "opencorporates.com"},
		{ID: "panjiva", Name: "Panjiva", Strategy: 4, URL: "panjiva.com"},
		{ID: "stateinfoservices", Name: "State Info Services", Strategy: 4, URL: "stateinformationservices.com"},
		{ID: "realyellowpages", Name: "The Real Yellow Pages", Strategy: 4, URL: "yellowpages.com"},
		{ID: "wacompanysearch", Name: "WA Company Search", Strategy: 4, URL: "ccfs.sos.wa.gov"},
		{ID: "wealthminder", Name: "Wealthminder", Strategy: 4, URL: "wealthminder.com"},
		{ID: "websiteoutlook", Name: "WebsiteOutlook", Strategy: 4, URL: "websiteoutlook.com"},
		{ID: "zaubee", Name: "Zaubee", Strategy: 4, URL: "zaubee.com"},
		{ID: "localchiros", Name: "localchiros.com", Strategy: 4, URL: "localchiros.com"},
		{ID: "ppploaninfo", Name: "ppp-loan.info", Strategy: 4, URL: "ppp-loan.info"},

		// ── Strategy 5: Phone Directories via WHOIS (12 sites) ───────────────────
		{ID: "1called", Name: "1called.com", Strategy: 5, URL: "1called.com"},
		{ID: "1whonet", Name: "1who.net", Strategy: 5, URL: "1who.net"},
		{ID: "411reverselookup", Name: "411reverselookup.ca", Strategy: 5, URL: "411reverselookup.ca"},
		{ID: "calleridtest", Name: "CallerIDTest", Strategy: 5, URL: "calleridtest.com"},
		{ID: "phonecheckpro", Name: "PhoneCheck Pro", Strategy: 5, URL: "phonecheckpro.com"},
		{ID: "phonehistory", Name: "PhoneHistory.com", Strategy: 5, URL: "phonehistory.com"},
		{ID: "processingbordeaux", Name: "ProcessingBordeaux", Strategy: 5, URL: "processingbordeaux.com"},
		{ID: "reverselookups", Name: "Reverselookups.org", Strategy: 5, URL: "reverselookups.org"},
		{ID: "usphonepro", Name: "US Phone Pro", Strategy: 5, URL: "usphonepro.com"},
		{ID: "usphonelookup", Name: "USPhoneLookup", Strategy: 5, URL: "usphonelookup.com"},
		{ID: "validnumber", Name: "Valid Number", Strategy: 5, URL: "validnumber.com"},
		{ID: "whoseno", Name: "Whoseno", Strategy: 5, URL: "whoseno.com"},

		// ── Strategy 6: Profile Brokers, Manual Research (9 sites) ───────────────
		{ID: "bradylist", Name: "Brady List", Strategy: 6, URL: "bradylist.com"},
		{ID: "docplayer", Name: "DocPlayer Inc.", Strategy: 6, URL: "docplayer.net"},
		{ID: "identiq", Name: "Identiq", Strategy: 6, URL: "identiq.com"},
		{ID: "joblookup", Name: "JobLookup Ltd", Strategy: 6, URL: "joblookup.com"},
		{ID: "newcon", Name: "Newcon", Strategy: 6, URL: "newcon.com"},
		{ID: "prehired", Name: "Prehired", Strategy: 6, URL: "prehired.io"},
		{ID: "slideplayer", Name: "SlidePlayer", Strategy: 6, URL: "slideplayer.com"},
		{ID: "usvisainfo", Name: "US Visa Info Service", Strategy: 6, URL: "usvisainformation.com"},
		{ID: "facecheckid", Name: "FaceCheck.ID", Strategy: 6, URL: "facecheck.id"},
	}
}
