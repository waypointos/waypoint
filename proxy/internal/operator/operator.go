package operator

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// SessionJWTTTL is the lifetime of session user JWTs minted by
// MintSessionUserJWT. Dashboard refresh logic refreshes well before this.
const SessionJWTTTL = 1 * time.Hour

// Operator holds the root signing key for the Waypoint NATS hub.
// One operator key is generated once per deployment and stored as a base64
// NKey seed in the OPERATOR_NKEY_SEED env var (or proxy/operator.nk for dev).
type Operator struct {
	kp nkeys.KeyPair

	// accountKPs caches account keypairs by account pubkey so that
	// MintRoverUserJWT can sign with the correct account key.
	accountMu  sync.Mutex
	accountKPs map[string]nkeys.KeyPair

	// sessionKP is the dedicated "sessions" account keypair under which all
	// dashboard session user JWTs are issued. The jwt/v2 library requires user
	// JWTs to be signed by an account key (not an operator key), and session
	// JWTs need to span multiple rovers — so they cannot reuse a per-rover
	// account. Derived deterministically from the operator seed so that
	// LoadFromSeed produces the same session account pubkey across restarts;
	// otherwise the embedded NATS server's trusted-account list would need
	// re-syncing on every proxy boot.
	sessionKP nkeys.KeyPair
}

func New() (*Operator, error) {
	kp, err := nkeys.CreateOperator()
	if err != nil {
		return nil, fmt.Errorf("create operator: %w", err)
	}
	return newWithKP(kp)
}

func LoadFromSeed(seed []byte) (*Operator, error) {
	kp, err := nkeys.FromSeed(seed)
	if err != nil {
		return nil, fmt.Errorf("from seed: %w", err)
	}
	return newWithKP(kp)
}

// newWithKP wraps an operator keypair and derives the deterministic
// session-account keypair from the operator seed.
func newWithKP(kp nkeys.KeyPair) (*Operator, error) {
	sessionKP, err := deriveSessionAccountKP(kp)
	if err != nil {
		return nil, fmt.Errorf("derive session account: %w", err)
	}
	return &Operator{
		kp:         kp,
		accountKPs: make(map[string]nkeys.KeyPair),
		sessionKP:  sessionKP,
	}, nil
}

// deriveSessionAccountKP produces a stable account keypair from the operator's
// raw seed. SHA-256 the operator seed with a domain-separation tag to get 32
// bytes of entropy, then build an nkeys account seed from it.
func deriveSessionAccountKP(opKP nkeys.KeyPair) (nkeys.KeyPair, error) {
	opSeed, err := opKP.Seed()
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write([]byte("waypoint:sessions-account:v1"))
	h.Write(opSeed)
	sum := h.Sum(nil) // 32 bytes
	return nkeys.FromRawSeed(nkeys.PrefixByteAccount, sum)
}

func (o *Operator) PublicKey() string {
	pk, _ := o.kp.PublicKey()
	return pk
}

func (o *Operator) Seed() ([]byte, error) { return o.kp.Seed() }

// SigningKey returns the operator's raw nkeys.KeyPair, for use by packages
// (e.g. magiclink) that need to sign with the operator key directly.
func (o *Operator) SigningKey() nkeys.KeyPair { return o.kp }

// SessionAccountPublicKey returns the public key of the sessions account under
// which all dashboard session user JWTs are issued. The embedded NATS server
// must trust this account (i.e. its account JWT must be registered with the
// account resolver) so that session JWTs presented over WebSocket validate.
func (o *Operator) SessionAccountPublicKey() string {
	pk, _ := o.sessionKP.PublicKey()
	return pk
}

// RoverImport identifies one rover account whose published subjects
// should be imported into the sessions account. The proxy's internal
// subscribers (heartbeat consumer, audit subscriber, alerts mirror) all
// connect under the sessions account, so without these imports they
// cannot see rover publishes — NATS enforces per-account isolation.
type RoverImport struct {
	// RoverID is used to build the imported subject (waypoint.<id>.>)
	// and to label the import for diagnostic logs.
	RoverID string
	// AccountPubKey is the rover account's NKey pubkey — the import's
	// "Account" field. nats-server matches an import's source to the
	// exporting account by pubkey, not by name.
	AccountPubKey string
}

// MintSessionAccountJWT mints an account JWT for the dedicated sessions account
// under which all dashboard session user JWTs and proxy-internal subscribers
// are issued. The returned JWT must be registered with the embedded NATS
// server's account resolver (see natshub.Hub.RegisterAccount) before any
// user can authenticate.
//
// allowedSubjects is added to the account's DefaultPermissions for both pub and
// sub — these are the upper bound; per-user JWTs minted via MintSessionUserJWT
// further narrow this set per-rover/per-role.
//
// roverImports adds one Stream Import per rover so the internal subscribers
// can receive rover publishes across the per-rover account boundary. Each
// rover account must export the matching subject (see MintAccountJWT). The
// imports list must be refreshed (and the JWT re-registered) whenever a
// rover is enrolled or removed.
//
// The account is signed by the operator key (operator may sign account claims).
func (o *Operator) MintSessionAccountJWT(allowedSubjects []string, roverImports []RoverImport) (string, error) {
	apk, err := o.sessionKP.PublicKey()
	if err != nil {
		return "", err
	}
	ac := jwt.NewAccountClaims(apk)
	ac.Name = "waypoint:sessions"
	ac.Issuer = o.PublicKey()
	ac.IssuedAt = time.Now().Unix()
	ac.DefaultPermissions.Pub.Allow.Add(allowedSubjects...)
	ac.DefaultPermissions.Sub.Allow.Add(allowedSubjects...)
	for _, ri := range roverImports {
		ac.Imports = append(ac.Imports, &jwt.Import{
			Name:    "rover:" + ri.RoverID + ":subjects",
			Subject: jwt.Subject("waypoint." + ri.RoverID + ".>"),
			Account: ri.AccountPubKey,
			Type:    jwt.Stream,
		})
	}
	return ac.Encode(o.kp)
}

// MintAccountJWT creates a new NATS account JWT for the given rover and
// returns the JWT, the account's public key (used as the DB pkey), and
// the account NKey seed. The account is restricted to allowedSubjects.
//
// The account keypair is cached internally so MintRoverUserJWT can sign
// user JWTs with the correct account key. The seed is also returned so
// callers (the enroll handler) can persist it alongside the JWT —
// without that, every proxy restart wipes the cache and the operator
// can no longer mint additional user JWTs under this account. The
// startup path uses RestoreAccountJWT to re-hydrate the cache and
// re-derive the JWT under current claims.
//
// The minted account JWT carries a Stream export on
// "waypoint.<roverID>.>" so the sessions account (used by the proxy's
// internal subscribers — heartbeat, audit, alerts) can import the
// rover's published subjects. Without it, account isolation prevents
// the heartbeat consumer from seeing the rover's publishes and
// rovers.last_seen_at would never update.
func (o *Operator) MintAccountJWT(roverID string, allowedSubjects []string) (jwtTok, pubKey string, seed []byte, err error) {
	akp, err := nkeys.CreateAccount()
	if err != nil {
		return "", "", nil, err
	}
	akSeed, err := akp.Seed()
	if err != nil {
		return "", "", nil, err
	}
	tok, apk, err := o.encodeAccountJWT(roverID, akp, allowedSubjects)
	if err != nil {
		return "", "", nil, err
	}
	return tok, apk, akSeed, nil
}

// RestoreAccountJWT rebuilds a rover account JWT from a previously-minted
// seed and re-populates the internal keypair cache. The minted JWT
// reflects the current code's claims structure (which is why startup
// uses this path instead of replaying the persisted JWT verbatim — that
// way exports added since a rover was originally enrolled propagate on
// the next proxy restart without operator intervention).
func (o *Operator) RestoreAccountJWT(roverID string, seed []byte, allowedSubjects []string) (jwtTok, pubKey string, err error) {
	akp, err := nkeys.FromSeed(seed)
	if err != nil {
		return "", "", fmt.Errorf("from seed: %w", err)
	}
	return o.encodeAccountJWT(roverID, akp, allowedSubjects)
}

// encodeAccountJWT is the shared body of MintAccountJWT + RestoreAccountJWT.
// Builds the AccountClaims, encodes them with the operator signing key,
// and caches the keypair for later user-JWT minting.
func (o *Operator) encodeAccountJWT(roverID string, akp nkeys.KeyPair, allowedSubjects []string) (string, string, error) {
	apk, err := akp.PublicKey()
	if err != nil {
		return "", "", err
	}

	ac := jwt.NewAccountClaims(apk)
	ac.Name = "rover:" + roverID
	ac.Issuer = o.PublicKey()
	ac.IssuedAt = time.Now().Unix()
	ac.DefaultPermissions.Pub.Allow.Add(allowedSubjects...)
	ac.DefaultPermissions.Sub.Allow.Add(allowedSubjects...)
	// _INBOX.> on the account default so a rover user minted with empty perms
	// still carries intra-account request/reply over the leaf — the hub's
	// _INBOX.<nuid> interest propagates down (sub) and the agent's reply goes
	// back up (pub). It must live here, not on the user: a user with any
	// explicit allow entry stops inheriting the account-default allow-list.
	ac.DefaultPermissions.Pub.Allow.Add("_INBOX.>")
	ac.DefaultPermissions.Sub.Allow.Add("_INBOX.>")
	// Resp authorises a singleton reply to RPC requests the rover receives over
	// the leaf (e.g. camera_offer). On the account default so an empty-perms
	// rover user inherits it.
	ac.DefaultPermissions.Resp = &jwt.ResponsePermission{MaxMsgs: 1}
	// Stream export of the rover's own subject namespace. The sessions
	// account imports this so the proxy's internal subscribers can see
	// heartbeat/telemetry publishes despite the per-rover account boundary.
	ac.Exports = jwt.Exports{
		&jwt.Export{
			Name:    "rover:" + roverID + ":subjects",
			Subject: jwt.Subject("waypoint." + roverID + ".>"),
			Type:    jwt.Stream,
		},
	}

	tok, err := ac.Encode(o.kp)
	if err != nil {
		return "", "", err
	}

	o.accountMu.Lock()
	o.accountKPs[apk] = akp
	o.accountMu.Unlock()

	return tok, apk, nil
}

// RestoreAccountKP rebuilds an account keypair from a previously-minted
// seed and re-populates the internal cache used by MintRoverUserJWT.
// Returns the account public key so callers can sanity-check it matches
// what they persisted alongside the seed.
//
// Used at proxy startup to recover from the in-memory cache being wiped
// on process restart — for every rover row in the DB, call this with
// its stored seed and then RegisterAccount the stored JWT with the hub.
func (o *Operator) RestoreAccountKP(seed []byte) (string, error) {
	akp, err := nkeys.FromSeed(seed)
	if err != nil {
		return "", fmt.Errorf("from seed: %w", err)
	}
	apk, err := akp.PublicKey()
	if err != nil {
		return "", err
	}
	o.accountMu.Lock()
	o.accountKPs[apk] = akp
	o.accountMu.Unlock()
	return apk, nil
}

// MintRevokedAccountJWT produces an account JWT for the given account public
// key with no permissions and Expires set in the past, signed by the operator.
// Storing this in the embedded NATS resolver (via Hub.RevokeAccount) overrides
// the original account JWT — new connections are rejected on IsExpired, and an
// UpdateAccountClaims call against the in-memory account drops permissions for
// any client that re-evaluates ACLs.
//
// The account keypair is never needed here because we are not minting user
// JWTs under it; the operator key is the legitimate issuer of account claims.
func (o *Operator) MintRevokedAccountJWT(accountPubKey string) (string, error) {
	ac := jwt.NewAccountClaims(accountPubKey)
	ac.Name = "rover:revoked"
	ac.Issuer = o.PublicKey()
	now := time.Now()
	ac.IssuedAt = now.Unix()
	ac.Expires = now.Add(-time.Minute).Unix()
	// No permissions: empty Pub.Allow / Sub.Allow plus a deny-all in Pub
	// closes both directions even on servers that fall back to defaults.
	ac.DefaultPermissions.Pub.Deny.Add(">")
	ac.DefaultPermissions.Sub.Deny.Add(">")

	tok, err := ac.Encode(o.kp)
	if err != nil {
		return "", err
	}

	// Drop the cached account keypair — no further user JWTs should ever be
	// minted under this account.
	o.accountMu.Lock()
	delete(o.accountKPs, accountPubKey)
	o.accountMu.Unlock()

	return tok, nil
}

// MintRoverUserJWT mints a user JWT under the given account, returning the JWT
// and the user NKey seed (which the rover writes into its identity.toml).
// The user JWT is signed with the account keypair (cached from MintAccountJWT),
// which is what the NATS server and jwt library require.
func (o *Operator) MintRoverUserJWT(accountJWT string, allowedSubjects []string) (userJWT string, userSeed []byte, err error) {
	ac, err := jwt.DecodeAccountClaims(accountJWT)
	if err != nil {
		return "", nil, err
	}

	// Look up the cached account keypair so we can sign with it.
	o.accountMu.Lock()
	akp, ok := o.accountKPs[ac.Subject]
	o.accountMu.Unlock()
	if !ok {
		return "", nil, fmt.Errorf("account keypair not found for %s — call MintAccountJWT first", ac.Subject)
	}

	ukp, err := nkeys.CreateUser()
	if err != nil {
		return "", nil, err
	}
	upk, err := ukp.PublicKey()
	if err != nil {
		return "", nil, err
	}
	seed, err := ukp.Seed()
	if err != nil {
		return "", nil, err
	}

	uc := jwt.NewUserClaims(upk)
	uc.Name = "rover:user"
	uc.IssuerAccount = ac.Subject
	uc.IssuedAt = time.Now().Unix()
	// Empty allowedSubjects → mint with NO explicit permissions so the user
	// inherits the rover account's DefaultPermissions (subjects + _INBOX.> +
	// Resp). nats-server only falls back to account defaults when the user's
	// allow-lists are empty, so the inheriting path must add nothing here. A
	// non-empty list keeps the legacy explicit behaviour (used by older tests).
	if len(allowedSubjects) > 0 {
		uc.Permissions.Pub.Allow.Add(allowedSubjects...)
		uc.Permissions.Sub.Allow.Add(allowedSubjects...)
		uc.Permissions.Pub.Allow.Add("_INBOX.>")
		uc.Permissions.Sub.Allow.Add("_INBOX.>")
		uc.Permissions.Resp = &jwt.ResponsePermission{MaxMsgs: 1}
	}

	// Sign with the account keypair — required by the jwt library (UserClaims
	// ExpectedPrefixes only accepts account keys, not operator keys).
	tok, err := uc.Encode(akp)
	if err != nil {
		return "", nil, err
	}
	return tok, seed, nil
}

// MintRoverInternalUserJWT mints a user JWT under the rover's OWN account for
// the proxy's per-rover client connection. Issuing proxy→rover RPC/publishes
// from inside the rover account keeps them intra-account, so the leaf carries
// them both ways — unlike a cross-account Service import, which is not
// delivered to a leaf-backed responder. Permissions are explicit because this
// is a runtime proxy credential regenerated each boot (no re-enroll cost).
func (o *Operator) MintRoverInternalUserJWT(accountPubKey, roverID string) (string, []byte, error) {
	o.accountMu.Lock()
	akp, ok := o.accountKPs[accountPubKey]
	o.accountMu.Unlock()
	if !ok {
		return "", nil, fmt.Errorf("account keypair not found for %s — call MintAccountJWT/RestoreAccountKP first", accountPubKey)
	}

	ukp, err := nkeys.CreateUser()
	if err != nil {
		return "", nil, err
	}
	upk, err := ukp.PublicKey()
	if err != nil {
		return "", nil, err
	}
	seed, err := ukp.Seed()
	if err != nil {
		return "", nil, err
	}

	base := "waypoint." + roverID + ".>"
	uc := jwt.NewUserClaims(upk)
	uc.Name = "rover-internal:" + roverID
	uc.IssuerAccount = accountPubKey
	uc.IssuedAt = time.Now().Unix()
	// _INBOX.> for nats.go request/reply auto-inboxes; the rover namespace for
	// the request publish and any direct publishes (e.g. modules.desired).
	uc.Permissions.Pub.Allow.Add(base, "_INBOX.>")
	uc.Permissions.Sub.Allow.Add(base, "_INBOX.>")

	tok, err := uc.Encode(akp)
	if err != nil {
		return "", nil, err
	}
	return tok, seed, nil
}

// SessionAccess is one row of a user's rover access list, used as input to
// MintSessionUserJWT to encode per-rover pub/sub permissions into the JWT.
type SessionAccess struct {
	RoverID string
	Role    string // "monitor", "control", "admin"
}

// MintSessionUserJWT mints a short-lived NATS user JWT whose subject permissions
// encode the user's accesses across all granted rovers. Used by the dashboard's
// WebSocket gateway to enforce ACLs at the NATS protocol level.
//
// The JWT is issued under the operator's dedicated sessions account
// (see SessionAccountPublicKey) — the jwt/v2 library requires user JWTs to be
// signed by an account key, and session JWTs need to span multiple rovers, so
// they cannot reuse a per-rover account.
//
// The returned seed is the ephemeral NKey for this session; the dashboard
// presents (jwt, seed) when connecting. Both are short-lived; refresh by
// re-calling this method when the JWT nears expiry.
func (o *Operator) MintSessionUserJWT(userID string, accesses []SessionAccess, ttl time.Duration) (string, []byte, error) {
	ukp, err := nkeys.CreateUser()
	if err != nil {
		return "", nil, err
	}
	upk, err := ukp.PublicKey()
	if err != nil {
		return "", nil, err
	}
	seed, err := ukp.Seed()
	if err != nil {
		return "", nil, err
	}

	sessionAPK, err := o.sessionKP.PublicKey()
	if err != nil {
		return "", nil, err
	}

	uc := jwt.NewUserClaims(upk)
	uc.Name = "session:" + userID
	uc.IssuerAccount = sessionAPK
	uc.IssuedAt = time.Now().Unix()
	uc.Expires = uc.IssuedAt + int64(ttl.Seconds())

	for _, a := range accesses {
		base := "waypoint." + a.RoverID
		// All roles get the rover's full namespace on the sub side. The
		// rover account already exports `waypoint.<id>.>` as a single
		// Stream Export, and the sessions account imports the same
		// wildcard — narrowing the per-user JWT below that boundary just
		// produces Subscription Violations for any pane that subscribes
		// to a leaf subject the enumeration forgot (e.g. LogsPanel
		// subscribes to `waypoint.<id>.>` for its firehose pane and to
		// `waypoint.<id>.diag.log` for structured agent logs — neither
		// fits the telemetry/event/alert/infra/module enumeration that
		// used to live here). Pub permissions stay tight per role.
		uc.Permissions.Sub.Allow.Add(base + ".>")
		switch a.Role {
		case "control":
			uc.Permissions.Pub.Allow.Add(base + ".cmd.>")
			// Browser-initiated control RPCs (fire-and-forget; result returns on
			// event.mode, already covered by the base + ".>" sub grant). Allow-list,
			// not rpc.> — camera_offer/apply_image stay proxy-initiated.
			uc.Permissions.Pub.Allow.Add(base+".rpc.set_mode", base+".rpc.estop", base+".rpc.recover")
			// Interactive module panels publish commands into their own subtree
			// (fire-and-forget; results come back on a module telemetry subject
			// the base + ".>" sub grant already covers). No _INBOX.> grant: the
			// control role stays request/reply-free, same as monitor.
			uc.Permissions.Pub.Allow.Add(base + ".module.>")
		case "admin":
			uc.Permissions.Pub.Allow.Add(base+".cmd.>", base+".rpc.>")
			uc.Permissions.Pub.Allow.Add(base + ".module.>")
			uc.Permissions.Sub.Allow.Add(base + ".rpc.>")
			// _INBOX.> covers NATS request/reply auto-inboxes. nats.go's
			// Request() publishes the request and subscribes to an
			// auto-generated _INBOX.<nuid> reply subject, so admins need
			// both pub and sub on this prefix to complete an RPC round-trip.
			uc.Permissions.Pub.Allow.Add("_INBOX.>")
			uc.Permissions.Sub.Allow.Add("_INBOX.>")
		}
		// "monitor": sub only, no pub additions.
	}

	// nats-server's buildPermissionsFromJwt only enforces a closed-list
	// publish ACL when Pub.Allow or Pub.Deny is non-empty. An empty Allow
	// alone falls back to the account default ("waypoint.>"), so a monitor
	// user could still publish. Adding `>` (the full-wildcard token) to
	// Deny denies every subject and flips the user into closed-list mode.
	if len(uc.Permissions.Pub.Allow) == 0 {
		uc.Permissions.Pub.Deny.Add(">")
	}

	tok, err := uc.Encode(o.sessionKP)
	if err != nil {
		return "", nil, err
	}
	return tok, seed, nil
}

// MintInternalUserJWT mints a user JWT for the proxy's own client connection
// to its embedded NATS hub. The hub runs in operator mode and rejects
// anonymous connects with "Authorization Violation"; the proxy needs to
// present a JWT just like any external client.
//
// Permissions are broad on purpose — the proxy's internal subscribers
// (heartbeat consumer, audit subscriber, alerts mirror, etc.) need to
// see every rover's traffic. Per-user ACLs are enforced separately on
// the dashboard's per-WebSocket connection (MintSessionUserJWT).
//
// ttl == 0 means "no expiry" per jwt/v2 semantics. The internal client
// lives for the process lifetime; refresh would just add restart-replay
// complexity for no security benefit on an operator-controlled key.
func (o *Operator) MintInternalUserJWT(ttl time.Duration) (string, []byte, error) {
	ukp, err := nkeys.CreateUser()
	if err != nil {
		return "", nil, err
	}
	upk, err := ukp.PublicKey()
	if err != nil {
		return "", nil, err
	}
	seed, err := ukp.Seed()
	if err != nil {
		return "", nil, err
	}

	sessionAPK, err := o.sessionKP.PublicKey()
	if err != nil {
		return "", nil, err
	}

	uc := jwt.NewUserClaims(upk)
	uc.Name = "proxy-internal"
	uc.IssuerAccount = sessionAPK
	uc.IssuedAt = time.Now().Unix()
	if ttl > 0 {
		uc.Expires = uc.IssuedAt + int64(ttl.Seconds())
	}
	uc.Permissions.Pub.Allow.Add("waypoint.>", "_INBOX.>")
	uc.Permissions.Sub.Allow.Add("waypoint.>", "_INBOX.>")

	tok, err := uc.Encode(o.sessionKP)
	if err != nil {
		return "", nil, err
	}
	return tok, seed, nil
}
