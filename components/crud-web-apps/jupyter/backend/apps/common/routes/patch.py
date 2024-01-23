import datetime as dt
#2024/01/23 YCL modify
from kubernetes import client, config
from flask import  Flask, request, jsonify
#2024/01/23 YCL modify
from werkzeug import exceptions

from kubeflow.kubeflow.crud_backend import api, decorators, logging

from .. import status
from . import bp

log = logging.getLogger(__name__)

STOP_ATTR = "stopped"
ISTEMPLATE_ATTE = "istemplate"
CUSTOMERIMAGENAME_ATTR = "customerImageName"
CUSTOMERIMAGEVERSION_ATTR = "customerImageVersion"
CUSTOMERCOURSENAME_ATTR = "customerCourseName"
ATTRIBUTES = set([STOP_ATTR, ISTEMPLATE_ATTE, CUSTOMERIMAGENAME_ATTR, CUSTOMERIMAGEVERSION_ATTR, CUSTOMERCOURSENAME_ATTR])


# Routes
@bp.route(
    "/api/namespaces/<namespace>/notebooks/<notebook>", methods=["PATCH"]
)
@decorators.request_is_json_type
def patch_notebook(namespace, notebook):
    request_body = request.get_json()
    log.info("Got body: %s", request_body)

    if request_body is None:
        raise exceptions.BadRequest("Request doesn't have a body.")

    # Ensure request has at least one valid command
    if not any(attr in ATTRIBUTES for attr in request_body.keys()):
        raise exceptions.BadRequest(
            "Request body must include at least one supported key: %s"
            % list(ATTRIBUTES)
        )

    # start/stop a notebook
    if STOP_ATTR in request_body:
        start_stop_notebook(namespace, notebook, request_body)

    if ISTEMPLATE_ATTE in request_body:
        enable_disable_template_notebook(namespace, notebook, request_body)

    if CUSTOMERIMAGENAME_ATTR in request_body:
        set_customer_image_name_notebook(namespace, notebook, request_body)

    if CUSTOMERIMAGEVERSION_ATTR in request_body:
        set_customer_image_version_notebook(namespace, notebook, request_body)

    if CUSTOMERCOURSENAME_ATTR in request_body:
        set_course_image_name_notebook(namespace, notebook, request_body)

    return api.success_response()

# helper functions
def set_customer_image_name_notebook(namespace, notebook, request_body):
    setname = request_body[CUSTOMERIMAGENAME_ATTR]

    patch_body = {}

    log.info("Set Notebook '%s/%s' %s = %s", namespace, notebook, CUSTOMERIMAGENAME_ATTR, setname)

    patch_body = {
        "metadata": {"annotations": {CUSTOMERIMAGENAME_ATTR: setname}}
    }

    log.info(
        "Sending PATCH to Notebook %s/%s: %s", namespace, notebook, patch_body
    )
    api.patch_notebook(notebook, namespace, patch_body)

def set_customer_image_version_notebook(namespace, notebook, request_body):
    setname = request_body[CUSTOMERIMAGEVERSION_ATTR]

    patch_body = {}

    log.info("Set Notebook '%s/%s' %s = %s", namespace, notebook, CUSTOMERIMAGEVERSION_ATTR, setname)

    patch_body = {
        "metadata": {"annotations": {CUSTOMERIMAGEVERSION_ATTR: setname}}
    }

    log.info(
        "Sending PATCH to Notebook %s/%s: %s", namespace, notebook, patch_body
    )
    api.patch_notebook(notebook, namespace, patch_body)

def set_course_image_name_notebook(namespace, notebook, request_body):
    setname = request_body[CUSTOMERCOURSENAME_ATTR]

    patch_body = {}

    log.info("Set Notebook '%s/%s' %s = %s", namespace, notebook, CUSTOMERCOURSENAME_ATTR, setname)

    patch_body = {
        "metadata": {"annotations": {CUSTOMERCOURSENAME_ATTR: setname}}
    }

    log.info(
        "Sending PATCH to Notebook %s/%s: %s", namespace, notebook, patch_body
    )
    api.patch_notebook(notebook, namespace, patch_body)

def enable_disable_template_notebook(namespace, notebook, request_body):
    istemplate = request_body[ISTEMPLATE_ATTE]

    patch_body = {}
    if istemplate:
        log.info("Enable Notebook as Template '%s/%s'", namespace, notebook)

        patch_body = {
            "metadata": {"labels": {"isTemplateName": "yes"}}
        }
    else:
        log.info("Disable Notebook as Template '%s/%s'", namespace, notebook)
        patch_body = {
            "metadata": {"labels": {"isTemplateName": "no"}}
        }

    log.info(
        "Sending PATCH to Notebook %s/%s: %s", namespace, notebook, patch_body
    )
    api.patch_notebook(notebook, namespace, patch_body)

# helper functions
def start_stop_notebook(namespace, notebook, request_body):
    stop = request_body[STOP_ATTR]

    patch_body = {}
    if stop:
        if notebook_is_stopped(namespace, notebook):
            raise exceptions.Conflict(
                "Notebook %s/%s is already stopped." % (namespace, notebook)
            )

        log.info("Stopping Notebook Server '%s/%s'", namespace, notebook)
        now = dt.datetime.now(dt.timezone.utc)
        timestamp = now.strftime("%Y-%m-%dT%H:%M:%SZ")

        patch_body = {
            "metadata": {"annotations": {status.STOP_ANNOTATION: timestamp}}
        }
    else:
        log.info("Starting Notebook Server '%s/%s'", namespace, notebook)
        patch_body = {
            "metadata": {"annotations": {status.STOP_ANNOTATION: None}}
        }

    log.info(
        "Sending PATCH to Notebook %s/%s: %s", namespace, notebook, patch_body
    )
    api.patch_notebook(notebook, namespace, patch_body)


def notebook_is_stopped(namespace, notebook):
    log.info(
        "Checking if Notebook %s/%s is already stopped", namespace, notebook,
    )
    notebook = api.get_notebook(notebook, namespace)
    annotations = notebook.get("metadata", {}).get("annotations", {})

    return status.STOP_ANNOTATION in annotations

#2024/01/23 YCL adding new email start
@bp.route("/api/namespaces/<namespace>/aps_vnc/<name>", methods=["PATCH"])
def modify_authorization(namespace, name):
    config.load_kube_config()
    custom_api = client.CustomObjectsApi() 
    authorization_policy = custom_api.get_namespaced_custom_object('security.istio.io', 'v1beta1', namespace, 'authorizationpolicies', name)

    values_to_add = request.json.get('values_to_add', [])
    print("Values before modification:", authorization_policy["spec"]["rules"][0]["when"][0]["values"])
    for new_value in values_to_add:
        if new_value in authorization_policy["spec"]["rules"][0]["when"][0]["values"]:
            continue
        else:
            # 添加新email
            authorization_policy["spec"]["rules"][0]["when"][0]["values"].extend(values_to_add)

    # 更新AuthorizationPolicy 
    custom_api.patch_namespaced_custom_object('security.istio.io', 'v1beta1', namespace, 'authorizationpolicies', name, authorization_policy)
    print("Values after modification:", authorization_policy["spec"]["rules"][0]["when"][0]["values"])
    print("############")
    print(values_to_add)

    return jsonify({"message": "Authorization modified successfully"}), 200
#2024/01/23 YCL adding new email end

#2024/01/23 YCL deleting new email start
@bp.route("/api/namespaces/<namespace>/aps_vnc_1/<name>", methods=["PATCH"])
def modify_authorizaiton_delete(namespace, name):
    config.load_kube_config()
    custom_api = client.CustomObjectsApi() 
    authorization_policy = custom_api.get_namespaced_custom_object('security.istio.io', 'v1beta1', namespace, 'authorizationpolicies', name)
    
    values_to_delete = request.json.get('values_to_delete', [])
    print("Received JSON data:", request.json)  # 添加这一行
    print("Values before modification:", authorization_policy["spec"]["rules"][0]["when"][0]["values"])
    for new_value in values_to_delete:
        #if new_value in authorization_policy["spec"]["rules"][0]["when"][0]["values"]:
            authorization_policy["spec"]["rules"][0]["when"][0]["values"].remove(new_value)
        #else:
            #continue
    length = len(authorization_policy["spec"]["rules"][0]["when"][0]["values"])
    if length != 0:
        # 更新AuthorizationPolicy 
        custom_api.patch_namespaced_custom_object('security.istio.io', 'v1beta1', namespace, 'authorizationpolicies', name, authorization_policy)
        print("Values after modification:", authorization_policy["spec"]["rules"][0]["when"][0]["values"])
        print("############")
        print(values_to_delete)
        return jsonify({"message": "Authorization modified successfully"}), 200
    else:
        print("zeroooooooooooo")
        custom_api.delete_namespaced_custom_object('security.istio.io', 'v1beta1', namespace, 'authorizationpolicies', name)
        return jsonify({"message": "Authorization deleted successfully"}), 200
    
#2024/01/23 YCL deleting new email end
