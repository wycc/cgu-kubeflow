from flask import Blueprint

bp = Blueprint("base_routes", __name__)

from . import delete, get, patch, auth_bridge, notebook_status  # noqa: F401, E402
