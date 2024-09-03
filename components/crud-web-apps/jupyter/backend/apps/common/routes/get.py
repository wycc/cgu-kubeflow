from kubeflow.kubeflow.crud_backend import api, logging

from flask import request
from .. import utils
from . import bp
import os
import traceback
import json

log = logging.getLogger(__name__)

SECURE_COOKIES = os.getenv("APP_SECURE_COOKIES", "true").lower() == "true"
DISABLE_AUTH = os.getenv("APP_DISABLE_AUTH", "false").lower() == "true"
USER_HEADER = os.getenv("USERID_HEADER", "kubeflow-userid")
USER_PREFIX = os.getenv("USERID_PREFIX", ":")


@bp.route("/api/config")
def get_config():
    config = utils.load_spawner_ui_config()
    return api.success_response("config", config)

@bp.route("/api/manager/<namespace>")
def get_manager(namespace):
    print("get_manager: ", namespace)
    if namespace is None:
        return "user"
    profile = api.get_profile(namespace)
    manager = "user"
    if profile is not None:
        try:
            manager = profile["metadata"]["annotations"]["manager"]
        except:
            manager = "user"

    contents = [manager]

    return api.success_response("manager", contents)

@bp.route("/api/namespaces/<namespace>/pvcs")
def get_pvcs(namespace):
    pvcs = api.list_pvcs(namespace).items
    data = [{"name": pvc.metadata.name,
             "size": pvc.spec.resources.requests["storage"],
             "mode": pvc.spec.access_modes[0]} for pvc in pvcs]

    return api.success_response("pvcs", data)


@bp.route("/api/namespaces/<namespace>/poddefaults")
def get_poddefaults(namespace):
    pod_defaults = api.list_poddefaults(namespace)

    # Return a list of (label, desc) with the pod defaults
    contents = []
    for pd in pod_defaults["items"]:
        label = list(pd["spec"]["selector"]["matchLabels"].keys())[0]
        if "desc" in pd["spec"]:
            desc = pd["spec"]["desc"]
        else:
            desc = pd["metadata"]["name"]

        contents.append({"label": label, "desc": desc})

    log.info("Found poddefaults: %s", contents)
    return api.success_response("poddefaults", contents)


@bp.route("/api/namespaces/<namespace>/notebooks")
def get_notebooks(namespace):

    log.info("Get %s notebook", namespace)

    notebooks = api.list_notebooks(namespace)["items"]
    contents = [utils.notebook_dict_from_k8s_obj(nb) for nb in notebooks]

    return api.success_response("notebooks", contents)

@bp.route("/api/namespaces/<namespace>/allnotebooks")
def get_all_notebooks(namespace):

    log.info("Get %s allnotebooks", namespace)

    notebooks = api.list_all_notebooks(namespace)["items"]
    contents = [utils.notebook_dict_from_k8s_obj(nb) for nb in notebooks]

    return api.success_response("notebooks", contents)

@bp.route("/api/gpus")
def get_gpu_vendors():
    """
    Return a list of GPU vendors for which at least one node has the necessary
    annotation required to schedule pods
    """
    frontend_config = utils.load_spawner_ui_config()
    gpus_value = frontend_config.get("gpus", {}).get("value", {})
    config_vendor_keys = [
        v.get("limitsKey", "") for v in gpus_value.get("vendors", [])
    ]

    # Get all of the different resources installed in all nodes
    installed_resources = set()
    nodes = api.list_nodes().items
    for node in nodes:
        installed_resources.update(node.status.capacity.keys())

    # Keep the vendors the key of which exists in at least one node
    available_vendors = installed_resources.intersection(config_vendor_keys)

    return api.success_response("vendors", list(available_vendors))

#get authorizationpolicy start
@bp.route("/api/namespaces/<namespace>/aps")
def get_all_aps(namespace):
    try:
        aps = api.list_all_authorizationpolicy(namespace)["items"]
        print(aps)
        contents = [ap for ap in aps]
        
    except:
        print("XXXXXXXXXXXXXXXXXXXX")
        print(traceback.format_exc())
        return api.success_response("authorizationpolicy", ["xxx"])  
    return api.success_response("authorizationpolicy", contents)
#get authorizationpolicy end

#2024 YC notebook access start#
@bp.route("/api/namespaces/<namespace>/aps-1/<notebook>/<url1>")
def get_notebooks_access(namespace,notebook,url1):
    aps = api.get_notebooks_access(namespace,notebook,url1)

    if "view" in url1:
        authorization_name = f"notebook-{notebook}-authorizationpolicy-view"
    else:
        authorization_name = f"notebook-{notebook}-authorizationpolicy-editable"
    print(authorization_name)
    print(url1)
    print("##############################")

    target_ap = None
    for ap in aps:
        if not isinstance(ap, dict):
            print("Received unexpected data format for an item in the list:", type(ap))
            continue
        if ap.get("metadata", {}).get("name") == authorization_name:
            target_ap = ap
            break
    if target_ap is not None:
        values = target_ap["spec"]["rules"][0]["when"][0]["values"]
        print(values)
        print("##############################")
        return values
    else:
       values = ["None"]
       return values
#2024 YC notebook access end#

#2024 YC get profile start#
@bp.route("/api/namespaces/<namespace>/aps-2")
def get_profile1(namespace):
    profiles = api.get_profile(namespace)
    annotations_json = json.loads(profiles['metadata']['annotations']['kubectl.kubernetes.io/last-applied-configuration'])
    email = annotations_json['spec']['owner']['name']
    print(email)
    return api.success_response("email", email)
#2024 YC get profile end#
