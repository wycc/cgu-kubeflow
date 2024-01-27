import { Injectable } from '@angular/core';
import { BackendService, SnackBarService } from 'kubeflow';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { catchError, map } from 'rxjs/operators';
import {
  NotebookResponseObject,
  JWABackendResponse,
  Config,
  PodDefault,
  NotebookFormObject,
  NotebookProcessedObject,
  PvcResponseObject,
  AuthorizationPolicyResponseObject
} from '../types';
@Injectable({
  providedIn: 'root',
})
export class JWABackendService extends BackendService {
  constructor(public http: HttpClient, public snackBar: SnackBarService) {
    super(http, snackBar);
  }

  // GET
  public getManager(namespace: string): Observable<string[]> {
    const url = `api/manager/${namespace}`;

    return this.http.get<JWABackendResponse>(url).pipe(
      catchError(error => this.handleError(error)),
      map(data => data.manager),
    );
  }

  public getNotebooks(namespace: string): Observable<NotebookResponseObject[]> {
    const url = `api/namespaces/${namespace}/notebooks`;

    return this.http.get<JWABackendResponse>(url).pipe(
      catchError(error => this.handleError(error)),
      map((resp: JWABackendResponse) => {
        // console.log("resp.usery", resp.user)
        return resp.notebooks;
      }),
    );
  }

  public getAllNotebooks(namespace: string): Observable<NotebookResponseObject[]> {
    const url = `api/namespaces/${namespace}/allnotebooks`;

    return this.http.get<JWABackendResponse>(url).pipe(
      catchError(error => this.handleError(error)),
      map((resp: JWABackendResponse) => {
        // console.log("resp.usery", resp.user)
        return resp.notebooks;
      }),
    );
  }
  
// 2024/01/21 YCL authorizationPolicy start//
public getAllAuthorizationPolicy(namespace): Observable<AuthorizationPolicyResponseObject[]> {
    const url = `api/namespaces/${namespace}/aps`;

    return this.http.get<JWABackendResponse>(url).pipe(
      catchError(error => this.handleError(error)),
      map((resp: JWABackendResponse) => {
        console.log('xxxxxx');
        return resp.authorizationpolicy;
      }),
    );
  }
// 2024/01/21 YCL authorizationPolicy end//
  
  public getConfig(): Observable<Config> {
    const url = `api/config`;

    return this.http.get<JWABackendResponse>(url).pipe(
      catchError(error => this.handleError(error)),
      map(data => {
        return data.config;
      }),
    );
  }

  public getVolumes(ns: string): Observable<PvcResponseObject[]> {
    // Get existing PVCs in a namespace
    const url = `api/namespaces/${ns}/pvcs`;

    return this.http.get<JWABackendResponse>(url).pipe(
      catchError(error => this.handleError(error)),
      map(data => {
        return data.pvcs;
      }),
    );
  }

  public getPodDefaults(ns: string): Observable<PodDefault[]> {
    // Get existing PodDefaults in a namespace
    const url = `api/namespaces/${ns}/poddefaults`;

    return this.http.get<JWABackendResponse>(url).pipe(
      catchError(error => this.handleError(error)),
      map(data => {
        return data.poddefaults;
      }),
    );
  }

  public getGPUVendors(): Observable<string[]> {
    // Get installed GPU vendors
    const url = `api/gpus`;

    return this.http.get<JWABackendResponse>(url).pipe(
      catchError(error => this.handleError(error)),
      map(data => data.vendors),
    );
  }

  // PATCH
  // Lance - Begin - 20230818
  public setCustomerParamNotebook(notebook: NotebookProcessedObject, jsonTag: string, jsonValue:string): Observable<string> {
    const name = notebook.name;
    const namespace = notebook.namespace;
    const url = `api/namespaces/${namespace}/notebooks/${name}`;

    var obj = {};
    obj[jsonTag] = jsonValue;
    return this.http.patch<JWABackendResponse>(url, obj).pipe(
      catchError(error => this.handleError(error)),
      map(_ => {
        return 'started';
      }),
    );
  }

  public enableTemplateNotebook(notebook: NotebookProcessedObject): Observable<string> {
    const name = notebook.name;
    const namespace = notebook.namespace;
    const url = `api/namespaces/${namespace}/notebooks/${name}`;

    return this.http.patch<JWABackendResponse>(url, { istemplate: true }).pipe(
      catchError(error => this.handleError(error)),
      map(_ => {
        return 'started';
      }),
    );
  }

  public disableTemplateNotebook(notebook: NotebookProcessedObject): Observable<string> {
    const name = notebook.name;
    const namespace = notebook.namespace;
    const url = `api/namespaces/${namespace}/notebooks/${name}`;

    return this.http.patch<JWABackendResponse>(url, { istemplate: false }).pipe(
      catchError(error => this.handleError(error)),
      map(_ => {
        return 'stopped';
      }),
    );
  }

  // Lance - End - 20230818

  // POST
  public createNotebook(notebook: NotebookFormObject): Observable<string> {
    const url = `api/namespaces/${notebook.namespace}/notebooks`;

    return this.http.post<JWABackendResponse>(url, notebook).pipe(
      catchError(error => this.handleError(error)),
      map(_ => {
        return 'posted';
      }),
    );
  }

  // PATCH
  public startNotebook(notebook: NotebookProcessedObject): Observable<string> {
    const name = notebook.name;
    const namespace = notebook.namespace;
    const url = `api/namespaces/${namespace}/notebooks/${name}`;

    return this.http.patch<JWABackendResponse>(url, { stopped: false }).pipe(
      catchError(error => this.handleError(error)),
      map(_ => {
        return 'started';
      }),
    );
  }

  public stopNotebook(notebook: NotebookProcessedObject): Observable<string> {
    const name = notebook.name;
    const namespace = notebook.namespace;
    const url = `api/namespaces/${namespace}/notebooks/${name}`;

    return this.http.patch<JWABackendResponse>(url, { stopped: true }).pipe(
      catchError(error => this.handleError(error, false)),
      map(_ => {
        return 'stopped';
      }),
    );
  }
  
  //2024/01/21 YCL createauthorizationpolicy start//
  public createAuthorization(namespace,nameValue,pathValue,userEmail): Observable<string> {
    const url2 = `api/namespaces/${namespace}/aps_vnc`;
  
    // Create an object with the 'name' parameter
    const requestBody = { name: nameValue, paths: pathValue, useremail: userEmail};
    
    return this.http.post<JWABackendResponse>(url2,requestBody).pipe(
      catchError(_ => {
        return 'error';
      }),
      map(_ => {
        return 'posted';
      }),
    );
  }
  //2024/01/21 YCL createauthorizationpolicy end//
  
  //2024/01/21 YCL deleteauthorizationpolicy start// 
  // DELETE
   public deleteauthorization(delete_name: string, namespace: string) {
    const url = `api/namespaces/${namespace}/aps_vnc/${delete_name}`;
    return this.http
      .delete<JWABackendResponse>(url)
      .pipe(catchError(error => this.handleError(error, false)));
  }
//2024/01/21 YCL deleteauthorizationpolicy end// 
  
// 2024/01/23 YCL add data start//
  public modify_authorizaiton(nameSpace,namevalue,adddata): Observable<string> {
  const url2 = `api/namespaces/${nameSpace}/aps_vnc/${namevalue}`;

  // Create an object with the 'name' parameter
  const requestBody = { values_to_add: adddata};
  
  return this.http.patch<JWABackendResponse>(url2,requestBody).pipe(
    catchError(_ => {
      return 'error';
    }),
    map(_ => {
      return 'posted';
    }),
  );
}
// 2024/01/23 YCL add data end//
  
// 2024/01/23 YCL delete data start//
public modify_authorizaiton_delete(nameSpace,namevalue,deletedata): Observable<string> {
  const url2 = `api/namespaces/${nameSpace}/aps_vnc_1/${namevalue}`;

  // Create an object with the 'name' parameter
  const requestBody = {values_to_delete: deletedata};
  
  return this.http.patch<JWABackendResponse>(url2,requestBody).pipe(
    catchError(_ => {
      return 'error';
    }),
    map(_ => {
      return 'posted';
    }),
  );
}
// 2024/01/23 YCL delete data end//
  
  // DELETE
  public deleteNotebook(namespace: string, name: string) {
    const url = `api/namespaces/${namespace}/notebooks/${name}`;

    return this.http
      .delete<JWABackendResponse>(url)
      .pipe(catchError(error => this.handleError(error, false)));
  }
}
