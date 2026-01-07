"""Notebook status check endpoint.

Provides a lightweight endpoint for the share/wait page to poll the
runtime status of a Notebook. Returns a JSON payload containing a
boolean `running` flag and the processed status object used by the
frontend.
"""

from kubeflow.kubeflow.crud_backend import api, logging
from . import bp
from .. import status as nb_status

from flask import request

log = logging.getLogger(__name__)

@bp.route("/api/notebook/status")
def notebook_status():
	"""Fixed-path endpoint that accepts `namespace` as a query param.

	This avoids dynamic path rewriting issues in some proxies/ingress.
	The notebook name is fixed to `editor`.
	"""
	ns = request.args.get("namespace")
	if not ns:
		log.warning("notebook_status called without namespace query param")
		return api.success_response("notebook_status", {"running": False, "processed_status": {}})

	name = "editor"
	try:
		nb = api.get_notebook(name, ns)
	except Exception as e:
		log.exception("Failed to fetch notebook %s/%s: %s", ns, name, e)
		return api.success_response("notebook_status", {"running": False, "processed_status": {}})

	processed = nb_status.process_status(nb)

	running = False
	try:
		phase = processed.get("phase") if isinstance(processed, dict) else None
		if phase == "Ready" or phase == "ready":
			running = True
	except Exception:
		running = False

	# If the notebook was stopped, try to start it by removing the stop annotation.
	# Do this proactively so the share/wait flow can bring the editor up.
	try:
		phase_val = processed.get("phase") if isinstance(processed, dict) else None
		if isinstance(phase_val, str) and phase_val.lower() == "stopped":
			log.info("Notebook %s/%s is stopped — attempting to start", ns, name)
			patch_body = {"metadata": {"annotations": {nb_status.STOP_ANNOTATION: None}}}
			try:
				api.patch_notebook(name, ns, patch_body)
				log.info("Sent patch to start Notebook %s/%s", ns, name)
			except Exception as e:
				log.exception("Failed to send start patch for Notebook %s/%s: %s", ns, name, e)
	except Exception:
		# Defensive: don't let start attempts break status endpoint
		log.exception("Error while checking/attempting to start Notebook %s/%s", ns, name)

	return api.success_response("notebook_status", {"running": running, "processed_status": processed})

