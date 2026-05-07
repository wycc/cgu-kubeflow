from .. import authz
from kubernetes import client, config
from . import custom_api, events, utils


def get_notebook(notebook, namespace):
    authz.ensure_authorized(
        "get", "kubeflow.org", "v1beta1", "notebooks", namespace
    )
    return custom_api.get_namespaced_custom_object(
        "kubeflow.org", "v1beta1", namespace, "notebooks", notebook
    )

def get_notebook_unsafe(notebook, namespace):
    return custom_api.get_namespaced_custom_object(
        "kubeflow.org", "v1beta1", namespace, "notebooks", notebook
    )


def create_notebook(notebook, namespace, dry_run=False):
    authz.ensure_authorized(
        "create", "kubeflow.org", "v1beta1", "notebooks", namespace
    )

    return custom_api.create_namespaced_custom_object(
        "kubeflow.org", "v1beta1", namespace, "notebooks", notebook,
        dry_run="All" if dry_run else None)


def list_notebooks(namespace):
    authz.ensure_authorized(
        "list", "kubeflow.org", "v1beta1", "notebooks", namespace
    )
    return custom_api.list_namespaced_custom_object(
        "kubeflow.org", "v1beta1", namespace, "notebooks"
    )



def delete_notebook(notebook, namespace):
    authz.ensure_authorized(
        "delete", "kubeflow.org", "v1beta1", "notebooks", namespace
    )
    return custom_api.delete_namespaced_custom_object(
        group="kubeflow.org",
        version="v1beta1",
        namespace=namespace,
        plural="notebooks",
        name=notebook,
        propagation_policy="Foreground",
    )


def patch_notebook(notebook, namespace, body):
    authz.ensure_authorized(
        "patch", "kubeflow.org", "v1beta1", "notebooks", namespace
    )

    return custom_api.patch_namespaced_custom_object(
        "kubeflow.org", "v1beta1", namespace, "notebooks", notebook, body
    )
def patch_notebook_unsafe(notebook, namespace, body):
    return custom_api.patch_namespaced_custom_object(
        "kubeflow.org", "v1beta1", namespace, "notebooks", notebook, body
    )


def list_notebook_events(notebook, namespace):

    field_selector = utils.events_field_selector("Notebook", notebook)

    return events.list_events(namespace, field_selector)

def list_all_notebooks(namespace):
    authz.ensure_authorized(
        "list", "kubeflow.org", "v1beta1", "notebooks", namespace
    )
    return custom_api.list_cluster_custom_object(
        "kubeflow.org", "v1beta1", "notebooks", label_selector="isTemplateName=yes"
    )

#2024 YC get notebook access start#
def get_notebooks_access(namespace,notebook,url1):
    #authz.ensure_authorized(
    #    "list", "security.istio.io", "v1beta1", "authorizationpolicies", namespace
    #)
    authorization_policies = custom_api.list_namespaced_custom_object(
        "security.istio.io", "v1beta1", namespace, "authorizationpolicies"
    )

    if authorization_policies is None:
        return []

    return authorization_policies.get("items", [])
#2024 YC get notebook access end#

#Create authorization policy start #
def create_authorization(authorization,namespace,dry_run=False):
    # authz.ensure_authorized(
    #    "create", "security.istio.io", "v1beta1", "authorizationpolicies", namespace
    #)
    #print(authorization)
    return custom_api.create_namespaced_custom_object(
        "security.istio.io", "v1beta1", namespace, "authorizationpolicies",authorization,
        dry_run="All" if dry_run else None)
#Create authorization policy end #  

#2024/01/20 Delete authorization policy start#
def delete_authorization(name,namespace):
    #authz.ensure_authorized(
    #    "delete", "security.istio.io", "v1beta1", "authorizationpolicies", namespace
    #)
    return custom_api.delete_namespaced_custom_object(
        group="security.istio.io",
        version="v1beta1",
        namespace=namespace,
        plural="authorizationpolicies",
        name=name
    ) 
#2024/01/20 Delete authorization policy end#

#2024/01/23 Modify authorization policy start, Lance modify 2025/02/28#
def modify_authorization_add(namespace, name, new_user):
    #authz.ensure_authorized(
    #    "patch", "security.istio.io", "v1beta1", "authorizationpolicies", namespace
    #)

    #return custom_api.patch_namespaced_custom_object(
    #    "security.istio.io", "v1beta1", namespace, "authorizationpolicies", name, body
    #)
    try:

        if not namespace or not name or not new_user:
            print("Missing required parameters")
            return

        # Fetch the existing AuthorizationPolicy
        auth_policy = custom_api.get_namespaced_custom_object(
            group="security.istio.io",
            version="v1",
            namespace=namespace,
            plural="authorizationpolicies",
            name=name
        )

        # Locate the 'values' field in `when`
        rules = auth_policy.get("spec", {}).get("rules", [])
        if not rules:
            raise ValueError("No rules found in AuthorizationPolicy")

        when_conditions = rules[0].get("when", [])
        for condition in when_conditions:
            if condition["key"] == "request.headers[kubeflow-userid]":
                # Add the new user email if it's not already in the list
                if new_user not in condition["values"]:
                    condition["values"].append(new_user)
                break
        else:
            # If no matching "when" condition is found, create a new one
            when_conditions.append({
                "key": "request.headers[kubeflow-userid]",
                "values": [new_user]
            })

        # Prepare the patch data
        patch_body = {"spec": {"rules": rules}}

        # Patch the AuthorizationPolicy
        updated_policy = custom_api.patch_namespaced_custom_object(
            group="security.istio.io",
            version="v1",
            namespace=namespace,
            plural="authorizationpolicies",
            name=name,
            body=patch_body
        )

        print( "AuthorizationPolicy updated")

    except client.exceptions.ApiException as e:
        print(f"Kubernetes API error: {e}")
    except Exception as e:
        print(f"Internal server error: {str(e)}")

#2024/01/23 Modify authorization policy end#

#2024/01/23 Modify authorization policy start, Lance modify 2025/02/28#
def modify_authorization_delete(namespace, name, user_to_remove):
    #authz.ensure_authorized(
    #    "patch", "security.istio.io", "v1beta1", "authorizationpolicies", namespace
    #)

    #return custom_api.patch_namespaced_custom_object(
    #    "security.istio.io", "v1beta1", namespace, "authorizationpolicies", name, body
    #)
    try:

        if not namespace or not name or not user_to_remove:
            print("Missing required parameters")
            return

        # Fetch the existing AuthorizationPolicy
        auth_policy = custom_api.get_namespaced_custom_object(
            group="security.istio.io",
            version="v1",
            namespace=namespace,
            plural="authorizationpolicies",
            name=name
        )

        # Locate the 'values' field in `when`
        rules = auth_policy.get("spec", {}).get("rules", [])
        if not rules:
            raise ValueError("No rules found in AuthorizationPolicy")

        when_conditions = rules[0].get("when", [])
        for condition in when_conditions:
            if condition["key"] == "request.headers[kubeflow-userid]":
                # Remove the user if it exists in the list
                if user_to_remove in condition["values"]:
                    condition["values"].remove(user_to_remove)
                    print(f"User {user_to_remove} removed.")
                else:
                    print(f"User {user_to_remove} not found.")
                break

        # Prepare the patch data
        patch_body = {"spec": {"rules": rules}}

        # Patch the AuthorizationPolicy
        updated_policy = custom_api.patch_namespaced_custom_object(
            group="security.istio.io",
            version="v1",
            namespace=namespace,
            plural="authorizationpolicies",
            name=name,
            body=patch_body
        )

        print("AuthorizationPolicy updated successfully.")

    except client.exceptions.ApiException as e:
        print(f"Kubernetes API error: {e}")
    except Exception as e:
        print(f"Internal server error: {str(e)}")
#2024/01/23 Modify authorization policy end#

# List all authorizationpolicy start//    
def list_all_authorizationpolicy(namespace):
    #authz.ensure_authorized(
    #    "list", "security.istio.io", "v1beta1", "authorizationpolicies", namespace
    #)
    return custom_api.list_namespaced_custom_object(
        "security.istio.io", "v1beta1", namespace, "authorizationpolicies"
    )
# List all authorizationpolicy end


