"""Auth bridge handler.

This endpoint implements a server-side "auth bridge" used by the
filebrowser frontend. Behaviour:

- Accepts a `target` query parameter (URL-encoded). If present and the
    resolved target is same-origin, perform a server-side redirect to that
	target (useful after the auth proxy has allowed the request).
- Otherwise render a tiny informational page with a link/script to start
	the oauth2-proxy flow at `/oauth2/start`.

This mirrors the behaviour of the Go `auth_bridge` used by filebrowser so
the frontend can open `/api/public/auth-bridge?target=...` and the backend
will either redirect to the target (when authenticated) or provide a
mechanism to start authentication.
"""

from kubeflow.kubeflow.crud_backend import logging
from flask import request, redirect, render_template_string, jsonify
from . import bp
import urllib.parse

log = logging.getLogger(__name__)


@bp.route("/api/auth-bridge")
def auth_bridge():
		"""Handle the auth bridge request.

		- If `target` is provided and resolves to the same origin as the
			current request, perform a server-side redirect to it.
		- Otherwise return an HTML page that links to `/oauth2/start` so the
			client can trigger the auth flow.
		"""
		# Log incoming request basics for debugging (helpful when logs are viewed
		# from the backend pod). This will show whether the `target` query
		# parameter actually reached the backend and what forwarded headers were set.
		try:
			log.info("auth-bridge request url=%s host=%s path=%s args=%s",
					 request.url, request.host, request.path, dict(request.args))
			log.info("auth-bridge headers X-Forwarded-Proto=%s X-Forwarded-Host=%s Host=%s",
					 request.headers.get("X-Forwarded-Proto"),
					 request.headers.get("X-Forwarded-Host"),
					 request.headers.get("Host"))
		except Exception:
			# Defensive: logging should not break the handler
			log.exception("Failed logging auth-bridge request info")

		tgt = request.args.get("target", "")

		# Quick debug endpoint: if caller includes ?debug=1 return JSON with
		# what the backend saw. This is useful to confirm the request reached
		# the backend and which headers/params were present.
		if request.args.get("debug") in ("1", "true", "True"):
			debug_info = {
				"url": request.url,
				"path": request.path,
				"target": tgt,
				"args": dict(request.args),
				"headers": {
					"X-Forwarded-Proto": request.headers.get("X-Forwarded-Proto"),
					"X-Forwarded-Host": request.headers.get("X-Forwarded-Host"),
					"Host": request.headers.get("Host"),
				},
			}
			return jsonify(debug_info)
		if tgt:
			# Try to decode the provided target and parse it. Keep decoded
			# available for client-side redirect if server-side redirect isn't used.
			decoded = urllib.parse.unquote_plus(tgt)
			try:
				parsed = urllib.parse.urlparse(decoded)

				# Determine the effective request origin using forwarded headers
				proto = request.headers.get("X-Forwarded-Proto", request.scheme)
				# Use X-Forwarded-Host if present (ingress/proxy), else Host, else request.host
				host_header = request.headers.get("X-Forwarded-Host") or request.headers.get("Host") or request.host
				base = f"{proto}://{host_header}"

				# If the URL is relative, resolve it against the request origin
				if not parsed.netloc:
					resolved = urllib.parse.urljoin(base, decoded)
					parsed = urllib.parse.urlparse(resolved)

				# Only redirect if the resolved target is same-origin (scheme+host)
				parsed_origin = f"{parsed.scheme}://{parsed.netloc}"
				req_origin = f"{proto}://{host_header}"
				log.debug("auth-bridge parsed target: %s, req_origin: %s", parsed_origin, req_origin)
				if parsed_origin.lower() == req_origin.lower():
					log.info("auth-bridge same-origin redirect to %s", parsed.geturl())
					return redirect(parsed.geturl(), code=302)
				# If not same-origin, fall through to client-side redirect below.
			except Exception:
				log.exception("Failed to parse/resolve auth-bridge target")

			# If we have a target but couldn't/shouldn't do a server-side redirect,
			# return an HTML page that opens the auth flow in a new tab and keeps
			# the bridge page visible so the user sees the status/instructions.
			# Compute oauth_start using forwarded headers so the client has the
			# correct ingress-origin to contact for oauth2/start.
			proto = request.headers.get("X-Forwarded-Proto", request.scheme)
			host_header = request.headers.get("X-Forwarded-Host") or request.headers.get("Host") or request.host
			origin = f"{proto}://{host_header.rstrip('/')}"
			oauth_start = origin + "/oauth2/start"
			log.info("auth-bridge serving client redirect page oauth_start=%s target=%s", oauth_start, decoded)
			client_html = """
			<!doctype html>
			<html>
			  <head>
			    <meta charset="utf-8" />
			    <title>登入中...</title>
			    <meta name="viewport" content="width=device-width, initial-scale=1" />
			    <style>body{{font-family:Arial,Helvetica,sans-serif;background:#f6f8fa;color:#333;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}}.card{{background:#fff;padding:24px;border-radius:8px;box-shadow:0 6px 18px rgba(0,0,0,.08);max-width:640px;text-align:center}}</style>
			  </head>
			  <body>
			    <div class="card">
			      <h2>需要登入</h2>
			      <p>將在新視窗開啟登入頁面。完成登入後，回到此頁並按下「已完成登入，返回目標」按鈕。</p>
			      <p style="margin:12px 0"><button id="openLogin" style="padding:10px 18px;background:#2979ff;color:white;border-radius:4px;border:0;cursor:pointer">開啟登入視窗</button></p>
			      <p style="margin-top:12px;color:#666;">如果新視窗被攔截，請允許彈出視窗或手動點擊。</p>
			      <p style="margin-top:18px"><button id="done" style="padding:10px 18px;background:#4caf50;color:white;border-radius:4px;border:0;cursor:pointer;display:none">已完成登入，返回目標</button></p>
			    </div>
			    <script>
			      const oauthStart = {{ oauth_start|tojson }};
			      const target = {{ target|tojson }};
			      const openLogin = document.getElementById('openLogin');
			      const doneBtn = document.getElementById('done');
					function openAuth() {{
						try {{
							const w = window.open(oauthStart, '_blank');
							if (w) {{
								// show the done button if a new window was opened
								doneBtn.style.display = 'inline-block';
								w.focus && w.focus();
							}} else {{
								// popup blocked — still show done button and let user click manual link
								doneBtn.style.display = 'inline-block';
							}}
						}} catch (e) {{
							doneBtn.style.display = 'inline-block';
						}}
					}}
				openLogin.addEventListener('click', openAuth);
				// Provide an explicit anchor the user can click if popups are blocked
				const manual = document.getElementById('manualLink');
				manual.style.display = 'inline-block';
			      doneBtn.addEventListener('click', function() {{
			        try {{ window.location.replace(target); }} catch(e) {{ window.location.href = target; }}
			      }});
			    </script>
			  </body>
			</html>
			"""
			return render_template_string(client_html, target=decoded, oauth_start=oauth_start)
