// btkAuthBase is where this page's login routes live. A page that mounts them
// elsewhere declares it once, as data-btk-auth-base on the body; every other
// page keeps the default and changes nothing. shortlink mounts under /_/auth,
// so that "api" does not become a slug nobody can ever take.
function btkAuthBase() {
    var body = document.body;
    var base = body && body.dataset ? body.dataset.btkAuthBase : '';
    return base || '/api/auth';
}

// btkLoginPath is the login page, declared as data-btk-login-path on the body
// by a page that moves it.
function btkLoginPath() {
    var body = document.body;
    var path = body && body.dataset ? body.dataset.btkLoginPath : '';
    return path || '/login';
}

// btkLoginHereURL addresses the login page so that it returns the visitor to
// this page.
function btkLoginHereURL() {
    return btkLoginPath() + '?next=' + encodeURIComponent(window.location.pathname + window.location.search);
}

// btkGoToLogin sends the visitor to the login page. On the login page itself
// there is nowhere better to go.
function btkGoToLogin() {
    if (window.location.pathname === btkLoginPath()) return;
    window.location.href = btkLoginHereURL();
}

// btkLogin signs in with the extension. onError, when given, receives the
// failure message in place of the alert, so a page can show it inline.
async function btkLogin(button, onSuccess, onError) {
    var btn = button || document.getElementById('btk-login-btn');
    var fail = function(msg) {
        if (btn) btn.disabled = false;
        if (typeof onError === 'function') {
            onError(msg);
            return;
        }
        alert('Login failed: ' + msg);
    };

    if (!window.nostr) {
        // Cancelable so a page can present its own no-signer UI by calling
        // preventDefault. Otherwise the login page offers the remote signers.
        var unhandled = document.dispatchEvent(new CustomEvent('btk:no-signer', { cancelable: true }));
        if (unhandled) btkGoToLogin();
        return;
    }

    if (btn) btn.disabled = true;

    try {
        var pubkey = await window.nostr.getPublicKey();

        var challengeResp = await fetch(btkAuthBase() + '/challenge', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ pubkey: pubkey })
        });
        if (!challengeResp.ok) {
            var cdata = await challengeResp.json().catch(function() { return {}; });
            throw new Error(cdata.error || 'Failed to get challenge');
        }

        var challengeEvent = await challengeResp.json();
        var signedEvent = await window.nostr.signEvent(challengeEvent);

        var navAuth = document.getElementById('nav-auth');
        var errorHandler = function(evt) {
            if (evt.detail.successful) {
                if (typeof onSuccess === 'function') onSuccess();
                return;
            }
            var xhr = evt.detail.xhr;
            var msg = 'Verification failed';
            if (xhr && xhr.responseText) {
                try {
                    var data = JSON.parse(xhr.responseText);
                    if (data.error) msg = data.error;
                } catch (e) {}
            }
            console.error('Login failed:', msg);
            fail(msg);
        };
        if (navAuth) {
            navAuth.addEventListener('htmx:afterRequest', errorHandler, { once: true });
        }

        htmx.ajax('POST', btkAuthBase() + '/verify', {
            target: '#nav-auth',
            swap: 'outerHTML',
            values: { event: JSON.stringify(signedEvent) }
        });
    } catch (err) {
        console.error('Login failed:', err);
        fail(err.message);
    }
}

async function btkLogout() {
    try {
        await fetch(btkAuthBase() + '/logout', { method: 'POST' });
    } catch (err) {
        console.error('Logout error:', err);
    }
    window.location.reload();
}

async function btkCheckSession() {
    try {
        var resp = await fetch(btkAuthBase() + '/me');
        if (resp.ok) {
            var html = await resp.text();
            var navAuth = document.getElementById('nav-auth');
            if (navAuth) {
                navAuth.outerHTML = html;
            }
        }
    } catch (err) {
        console.error('Session check error:', err);
    }
}

// The nav fragment marks a bunker-backed session. Read it at call time: the
// fragment hydrates through /me and can arrive after any script has run.
function btkSessionIsBunker() {
    var ids = ['nav-auth', 'mobile-nav-auth'];
    for (var i = 0; i < ids.length; i++) {
        var el = document.getElementById(ids[i]);
        if (el && el.getAttribute('data-signer') === 'bunker') return true;
    }
    return false;
}

// A 410 means the server no longer holds the bunker session, which a restart
// causes. A page may cancel btk:session-expired to show its own UI. Otherwise
// the cookie is cleared first, because the login page sends a signed-in
// visitor straight on, and the visitor goes to the login page.
function btkSessionExpired() {
    var unhandled = document.dispatchEvent(new CustomEvent('btk:session-expired', { cancelable: true }));
    if (unhandled) {
        fetch(btkAuthBase() + '/logout', { method: 'POST' })
            .catch(function(err) {
                console.error('Logout error:', err);
            })
            .then(btkGoToLogin);
    }
    return new Error('Your signing session ended. Sign in again.');
}

// How often btkSigner.ready looks for an extension.
var BTK_SIGNER_POLL_MS = 100;

// btkSigner is the one path for signing. A bunker-backed session signs on the
// server; every other session uses the extension.
var btkSigner = {
    // available reports whether this page can sign at all. A page checks it
    // before a gesture, never window.nostr, which a bunker session lacks.
    available() {
        return btkSessionIsBunker() || !!window.nostr;
    },

    // ready resolves true once this page can sign, and false after timeoutMs.
    // Extensions inject window.nostr asynchronously, so a page that needs a
    // signer at load waits here.
    ready(timeoutMs) {
        if (btkSigner.available()) return Promise.resolve(true);
        return new Promise(function(resolve) {
            var waited = 0;
            var timer = setInterval(function() {
                waited += BTK_SIGNER_POLL_MS;
                if (btkSigner.available()) {
                    clearInterval(timer);
                    resolve(true);
                } else if (waited >= timeoutMs) {
                    clearInterval(timer);
                    resolve(false);
                }
            }, BTK_SIGNER_POLL_MS);
        });
    },

    async getPublicKey() {
        if (btkSessionIsBunker()) {
            var resp = await fetch(btkAuthBase() + '/nip46/pubkey');
            if (resp.status === 410) throw btkSessionExpired();
            if (!resp.ok) throw new Error('Could not read your public key');
            var data = await resp.json();
            return data.pubkey;
        }
        if (!window.nostr) throw new Error('No signer available');
        return await window.nostr.getPublicKey();
    },

    async signEvent(event) {
        var plain = JSON.parse(JSON.stringify(event));

        if (btkSessionIsBunker()) {
            var resp = await fetch(btkAuthBase() + '/nip46/sign', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(plain)
            });
            if (resp.status === 410) throw btkSessionExpired();
            if (!resp.ok) throw new Error('Signing failed');
            return await resp.json();
        }

        if (!window.nostr) throw new Error('No signer available');
        // Some extensions return a Proxy, which serializes badly later.
        return JSON.parse(JSON.stringify(await window.nostr.signEvent(plain)));
    }
};

// The login page's extension button. It checks for window.nostr on click,
// because extensions inject it asynchronously.
function btkLoginWithExtension(button) {
    if (!window.nostr) {
        var missing = document.getElementById('btk-login-extension-missing');
        if (missing) missing.classList.remove('hidden');
        return;
    }
    var redirect = (button && button.dataset.redirect) || '/';
    btkLogin(button, function() {
        window.location.href = redirect;
    });
}

// The sign bridge lets the server sign relay AUTH as the signed-in visitor,
// whose extension lives only here. A page opts in with data-btk-sign-bridge
// on the body. The server streams each unsigned event, the page signs it
// through btkSigner, and posts it back. The page signs relay AUTH only.
var BTK_BRIDGE_KIND = 22242;

function btkStartSignBridge() {
    var body = document.body;
    if (!body || !body.dataset || body.dataset.btkSignBridge === undefined) return;
    if (typeof EventSource === 'undefined') return;

    var source = new EventSource(btkAuthBase() + '/bridge/requests');
    source.addEventListener('sign', function(msg) {
        var event;
        try {
            event = JSON.parse(msg.data);
        } catch (e) {
            return;
        }
        if (!event || event.kind !== BTK_BRIDGE_KIND) return;

        btkSigner.signEvent(event)
            .then(function(signed) {
                return fetch(btkAuthBase() + '/bridge/answer', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(signed)
                });
            })
            .then(function(resp) {
                if (resp.status === 401) source.close();
            })
            .catch(function(err) {
                console.error('Sign bridge:', err);
            });
    });
    // A dropped stream reconnects by itself. A refused one, such as a 401,
    // ends closed, and the bridge stops.
    source.onerror = function() {
        if (source.readyState === EventSource.CLOSED) source.close();
    };
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', btkStartSignBridge);
} else {
    btkStartSignBridge();
}
