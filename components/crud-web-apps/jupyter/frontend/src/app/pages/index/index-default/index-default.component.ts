import { Component, OnInit, OnDestroy, Input } from '@angular/core';
import { environment } from '@app/environment';
import {
  NamespaceService,
  ExponentialBackoff,
  ActionEvent,
  STATUS_TYPE,
  DialogConfig,
  ConfirmDialogService,
  SnackBarService,
  DIALOG_RESP,
  SnackType,
  ToolbarButton,
} from 'kubeflow';
import { JWABackendService } from 'src/app/services/backend.service';
import { Subscription } from 'rxjs';
import {
  defaultAdvancedConfig,
  defaultConfig,
  getDeleteDialogConfig,
  getStopDialogConfig,
  getDisableTemplateDialogConfig,
} from './config';
import { isEqual } from 'lodash';
import { NotebookResponseObject, NotebookProcessedObject } from 'src/app/types';
import { Router } from '@angular/router';
// YCL 2023/12/03 start
import { DialogSharing } from './dialog-sharing/dialog-sharing.component';
// YCL 2023/12/03 end
import { MatDialog } from '@angular/material/dialog';
import { AddPostDialogComponent } from './add-post-dialog/add-post-dialog.component';
import { AbstractControl } from '@angular/forms';


@Component({
  selector: 'app-index-default',
  templateUrl: './index-default.component.html',
  styleUrls: ['./index-default.component.scss'],
})
export class IndexDefaultComponent implements OnInit, OnDestroy {
  @Input()
  searchControl: AbstractControl;

  env = environment;
  poller: ExponentialBackoff;

  currNamespace = '';
  isBasic = false;
  configAdvance = defaultAdvancedConfig;
  config = defaultConfig;
  subs = new Subscription();

  currentName = '';
  currentField = '';

  //if (isBasicSetting) {
  //  this.config = defaultConfig;
  //} 
      
  rawData: NotebookResponseObject[] = [];
  processedData: NotebookProcessedObject[] = [];

  buttons: ToolbarButton[] = [
    new ToolbarButton({
      text: `New Notebook`,
      icon: 'add',
      stroked: true,
      fn: () => {
        this.router.navigate(['/new']);
      },
    }),
  ];

  constructor(
    public ns: NamespaceService,
    public backend: JWABackendService,
    public confirmDialog: ConfirmDialogService,
    public snackBar: SnackBarService,
    public router: Router,
    public dialog: MatDialog,
  ) {}

  search(event: any) {
      this.currentField = event;
      this.processedData = this.processIncomingData(this.rawData.filter((notebook) => {
        console.log(notebook.name);
        return (
          notebook.name.includes(this.currentField) ||
          notebook.namespace.includes(this.currentField) ||
          notebook.image.includes(this.currentField)
        );
      }));
      this.poller.reset();
  };
  ngOnInit(): void {
    this.poller = new ExponentialBackoff({ interval: 1000, retries: 3 });

    //this.backend.getUsername().subscribe(manager => {
    //  alert(manager);
    //});
    // alert( this.currNamespace );

    this.backend.getUsername().subscribe(username => {

      if (Object.keys(username).length === 0) {
        // Don't fire on empty config
        //console.log("NO username")
        this.isBasic = true;
        return;
      }

      if( username.substring(0,1) === "D" || username.substring(0,1) === "d" )
        this.isBasic = false;
      else
        this.isBasic = true;

      //this.isBasic = false;  
      //alert(username);
      //console.log("username", username)
    });

    // Poll for new data and reset the poller if different data is found
    this.subs.add(
      this.poller.start().subscribe(() => {
        if (!this.currNamespace) {
          return;
        }

        this.backend.getNotebooks(this.currNamespace).subscribe(notebooks => {
          if (!isEqual(this.rawData, notebooks)) {
            this.rawData = notebooks;

            // Update the frontend's state
            // this.processedData = this.processIncomingData(notebooks);
            this.processedData = this.processIncomingData(this.rawData.filter((notebook) => {
              console.log(notebook.name);
              return (
                notebook.name.includes(this.currentField) ||
                notebook.namespace.includes(this.currentField) ||
                notebook.image.includes(this.currentField)
              );
            }));
            this.poller.reset();
          }
        });
      }),
    );

    // Reset the poller whenever the selected namespace changes
    this.subs.add(
      this.ns.getSelectedNamespace().subscribe(ns => {
        this.currNamespace = ns;
        this.poller.reset();

        this.backend.getManager(this.currNamespace).subscribe(manager => {
            // alert(manager[0]);
            if( manager[0] === "manager" )
              this.isBasic = false;
            else
              this.isBasic = true;
        });
      }),
    );
  }

  ngOnDestroy() {
    this.subs.unsubscribe();
    this.poller.stop();
  }

  public viewClicked(notebook: NotebookProcessedObject) {
    window.open(`/notebook/${notebook.namespace}/${notebook.name}/view`);
  }
  
  public shareClicked(notebook: NotebookProcessedObject) {

  }
  
  // Event handling functions
  reactToAction(a: ActionEvent) {
    switch (a.action) {
      case 'template':
        this.templateClicked(a.data);
        break;
      case 'remove-template':
          this.templateClicked(a.data);
          break;        
      case 'delete':
        this.deleteVolumeClicked(a.data);
        /*
        this.backend.getUsername().subscribe(username => {
          if (Object.keys(username).length === 0) {
            // Don't fire on empty config
            console.log("NO username")
            alert("NO username");
            return;
          }
    
          console.log("username", username)
          alert(username);
        });
        */
        break;
      case 'connect':
        this.connectClicked(a.data);
        break;
      case 'start-stop':
        this.startStopClicked(a.data);
        break;
      case 'share':
        // this.shareClicked(a.data);
        const dialogRef =this.dialog.open(DialogSharing,{ data: { namespace: a.data.namespace, name: a.data.name }});  
        dialogRef.afterClosed().subscribe((result) => {
            //const jsyaml = require('js-yaml');
            if (result && result.useremail) {
              const useremail = result.useremail;
              console.log('User Email from Dialog:', useremail);
              const selected = result.selected;
              // if select "view" //
              if (selected =='option1'){
                const paths = `/notebook/${a.data.namespace}/${a.data.name}/view/*`;
                const namevalue = `notebook-${a.data.name}-authorizationpolicy-view`;

                this.backend.getAllAuthorizationPolicy(a.data.namespace).subscribe(aps => {
                  console.log(a.data.namespace);
                  var deletename = "notebook-" + a.data.name +"-authorizationpolicy-view";
                  var names = aps.map((ap) => { return ap.metadata.name });
                  var filteredNames = names.filter((name) => name.includes(deletename));
              
                if (filteredNames.length <= 0) {
                  this.backend.createAuthorization(this.currNamespace,namevalue,paths,useremail).subscribe(
                    (response) => {
                    console.log("Success");
                    console.log('currNamespace:', this.currNamespace);
                    console.log('namevalue:', namevalue);
                    console.log("paths:", paths);
                    console.log("useremail:", useremail);
                    console.log("selected-option:", selected);
                    },
                (error) => {
                  console.error('Error creating authorization policy:', error);
                });
                }else {
                  //2024/01/23 新增email功能 start
                  this.backend.modify_authorizaiton(this.currNamespace,namevalue,useremail).subscribe(
                    (response) => {
                    console.log("Success for adding");
                    console.log('currNamespace:', this.currNamespace);
                    console.log('namevalue:', namevalue);
                    console.log("useremail:", useremail);
                    console.log("selected-option:", selected);
                    },
                    (error) => {
                      console.log('filteredName != 0, existed');
                  });
              
                }
              });
              //2024/01/23 新增email功能 end
            }else{
              // if select "editable" //
              const paths = `/notebook/${a.data.namespace}/${a.data.name}/*`;
              const namevalue = `notebook-${a.data.name}-authorizationpolicy-editable`;
              this.backend.getAllAuthorizationPolicy(a.data.namespace).subscribe(aps => {
                console.log(a.data.namespace);
                var deletename = "notebook-" + a.data.name +"-authorizationpolicy-editable";
                var names = aps.map((ap) => { return ap.metadata.name });
                var filteredNames = names.filter((name) => name.includes(deletename));
              if (filteredNames.length <= 0) {
                this.backend.createAuthorization(this.currNamespace,namevalue,paths,useremail).subscribe(
                (response) => {
                  console.log("Success");
                  console.log('currNamespace:', this.currNamespace);
                  console.log('namevalue:', namevalue);
                  console.log("paths:", paths);
                  console.log("useremail:", useremail);
                  console.log("selected-option:", selected);
                },
                (error) => {
                  console.error('Error creating authorization policy:', error);
                });
              }else {
                //2024/01/23 新增email功能 start
                this.backend.modify_authorizaiton(this.currNamespace,namevalue,useremail).subscribe(
                  (response) => {
                  console.log("Success for adding");
                  console.log('currNamespace:', this.currNamespace);
                  console.log('namevalue:', namevalue);
                  console.log("useremail:", useremail);
                  console.log("selected-option:", selected);
                  },
                  (error) => {
                    console.log('filteredName != 0, existed');
                });
              }
            })};
            //2024/01/23 新增email功能 end
          }
        });
        // 2024/1/16 YC end //
        break;
      case 'view':
        this.viewClicked(a.data);
        break;
        
    }
  }

  public templateClicked(notebook: NotebookProcessedObject) {
    if (notebook.isTemplate === 'yes') {
      this.disableTemplateNotebook(notebook);
    } else {
      this.enableTemplateNotebook(notebook);
    }
  }

  public enableTemplateNotebook(notebook: NotebookProcessedObject) {
    this.snackBar.open(
      $localize`Set Notebook as template '${notebook.name}'...`,
      SnackType.Info,
      1000,
    );

    notebook.status.phase = STATUS_TYPE.WAITING;
    notebook.status.message = 'Set Notebook as template...';
    this.updateNotebookFields(notebook);

    this.backend.enableTemplateNotebook(notebook).subscribe(() => {
      this.poller.reset();
    });

    this.showAddPostDialog(notebook);
  }

  public disableTemplateNotebook(notebook: NotebookProcessedObject) {
    const stopDialogConfig = getDisableTemplateDialogConfig(notebook.name);
    const ref = this.confirmDialog.open(notebook.name, stopDialogConfig);
    const stopSub = ref.componentInstance.applying$.subscribe(applying => {
      if (!applying) {
        return;
      }

      // Close the open dialog only if the request succeeded
      this.backend.disableTemplateNotebook(notebook).subscribe({
        next: _ => {
          this.poller.reset();
          ref.close(DIALOG_RESP.ACCEPT);
        },
        error: err => {
          const errorMsg = err;
          stopDialogConfig.error = errorMsg;
          ref.componentInstance.applying$.next(false);
        },
      });

      // request has succeeded
      ref.afterClosed().subscribe(res => {
        stopSub.unsubscribe();
        if (res !== DIALOG_RESP.ACCEPT) {
          return;
        }

        this.snackBar.open(
          $localize`Disable Notebook as template '${notebook.name}'...`,
          SnackType.Info,
          1000,
        );

        notebook.status.phase = STATUS_TYPE.TERMINATING;
        notebook.status.message = 'Preparing to disable the Notebook as template...';
        this.updateNotebookFields(notebook);
      });
    });
  }

  public deleteVolumeClicked(notebook: NotebookProcessedObject) {
    const deleteDialogConfig = getDeleteDialogConfig(notebook.name);

    const ref = this.confirmDialog.open(notebook.name, deleteDialogConfig);
    const delSub = ref.componentInstance.applying$.subscribe(applying => {
      if (!applying) {
        return;
      }

      // Close the open dialog only if the DELETE request succeeded
      this.backend.deleteNotebook(this.currNamespace, notebook.name).subscribe({
        next: _ => {
          this.poller.reset();
          ref.close(DIALOG_RESP.ACCEPT);
        },
        error: err => {
          const errorMsg = err;
          deleteDialogConfig.error = errorMsg;
          ref.componentInstance.applying$.next(false);
        },
      });

      // DELETE request has succeeded
      ref.afterClosed().subscribe(res => {
        delSub.unsubscribe();
        if (res !== DIALOG_RESP.ACCEPT) {
          return;
        }

        notebook.status.phase = STATUS_TYPE.TERMINATING;
        notebook.status.message = 'Preparing to delete the Notebook...';
        this.updateNotebookFields(notebook);
      });
    });
  }
  public connectClicked(notebook: NotebookProcessedObject) {
    // Open new tab to work on the Notebook
    window.open(`/notebook/${notebook.namespace}/${notebook.name}/`);
    // this.showAddPostDialog(notebook);

    /*
    this.backend.setCustomerParamNotebook(notebook,'customerImageName', 'qqqqqqqq').subscribe(() => {
      this.poller.reset();
    });

    this.backend.setCustomerParamNotebook(notebook,'customerImageVersion', 'wwwwwwww').subscribe(() => {
      this.poller.reset();
    });

    this.backend.setCustomerParamNotebook(notebook,'customerCourseName', 'eeeeeeee').subscribe(() => {
      this.poller.reset();
    });
    */
  }

  public startStopClicked(notebook: NotebookProcessedObject) {
    if (notebook.status.phase === STATUS_TYPE.STOPPED) {
      this.startNotebook(notebook);
    } else {
      this.stopNotebook(notebook);
    }
  }

  public startNotebook(notebook: NotebookProcessedObject) {
    this.snackBar.open(
      $localize`Starting Notebook server '${notebook.name}'...`,
      SnackType.Info,
      3000,
    );

    notebook.status.phase = STATUS_TYPE.WAITING;
    notebook.status.message = 'Starting the Notebook Server...';
    this.updateNotebookFields(notebook);

    this.backend.startNotebook(notebook).subscribe(() => {
      this.poller.reset();
    });
  }

  public stopNotebook(notebook: NotebookProcessedObject) {
    const stopDialogConfig = getStopDialogConfig(notebook.name);
    const ref = this.confirmDialog.open(notebook.name, stopDialogConfig);
    const stopSub = ref.componentInstance.applying$.subscribe(applying => {
      if (!applying) {
        return;
      }

      // Close the open dialog only if the request succeeded
      this.backend.stopNotebook(notebook).subscribe({
        next: _ => {
          this.poller.reset();
          ref.close(DIALOG_RESP.ACCEPT);
        },
        error: err => {
          const errorMsg = err;
          stopDialogConfig.error = errorMsg;
          ref.componentInstance.applying$.next(false);
        },
      });

      // request has succeeded
      ref.afterClosed().subscribe(res => {
        stopSub.unsubscribe();
        if (res !== DIALOG_RESP.ACCEPT) {
          return;
        }

        this.snackBar.open(
          $localize`Stopping Notebook server '${notebook.name}'...`,
          SnackType.Info,
          3000,
        );

        notebook.status.phase = STATUS_TYPE.TERMINATING;
        notebook.status.message = 'Preparing to stop the Notebook Server...';
        this.updateNotebookFields(notebook);
      });
    });
  }

  // Data processing functions
  updateNotebookFields(notebook: NotebookProcessedObject) {
    notebook.setTemplateAction = this.processSetTemplateActionStatus(notebook);
    notebook.removeTemplateAction = this.processRemoveTemplateActionStatus(notebook);
    notebook.deleteAction = this.processDeletionActionStatus(notebook);
    notebook.connectAction = this.processConnectActionStatus(notebook);
    notebook.startStopAction = this.processStartStopActionStatus(notebook);
    notebook.shareAction = STATUS_TYPE.READY;
    notebook.viewAction = STATUS_TYPE.READY;
  }

  processIncomingData(notebooks: NotebookResponseObject[]) {
    const notebooksCopy = JSON.parse(
      JSON.stringify(notebooks),
    ) as NotebookProcessedObject[];

    for (const nb of notebooksCopy) {
      this.updateNotebookFields(nb);
    }
    return notebooksCopy;
  }

  // Action handling functions
  processSetTemplateActionStatus(notebook: NotebookProcessedObject) {

    if (notebook.jsonStr === null) 
      return STATUS_TYPE.TERMINATING;
    
    if (notebook.isTemplate !== 'yes') {
      return STATUS_TYPE.READY;
    }

    // Lance
    // if (notebook.status.phase !== STATUS_TYPE.TERMINATING) {
    //    return STATUS_TYPE.READY;
    //  }

    return STATUS_TYPE.TERMINATING;
  }

  processRemoveTemplateActionStatus(notebook: NotebookProcessedObject) {

    if (notebook.isTemplate === 'yes') {
      return STATUS_TYPE.READY;
    }

    // Lance
    // if (notebook.status.phase !== STATUS_TYPE.TERMINATING) {
    //    return STATUS_TYPE.READY;
    //  }

    return STATUS_TYPE.TERMINATING;
  }

  processDeletionActionStatus(notebook: NotebookProcessedObject) {
    if (notebook.status.phase !== STATUS_TYPE.TERMINATING) {
      return STATUS_TYPE.READY;
    }

    return STATUS_TYPE.TERMINATING;
  }

  processStartStopActionStatus(notebook: NotebookProcessedObject) {
    // Stop button
    if (notebook.status.phase === STATUS_TYPE.READY) {
      return STATUS_TYPE.UNINITIALIZED;
    }

    // Start button
    if (notebook.status.phase === STATUS_TYPE.STOPPED) {
      return STATUS_TYPE.READY;
    }

    // If it is terminating, then the action should be disabled
    if (notebook.status.phase === STATUS_TYPE.TERMINATING) {
      return STATUS_TYPE.UNAVAILABLE;
    }

    // If the Notebook is not Terminating, then always allow the stop action
    return STATUS_TYPE.UNINITIALIZED;
  }

  processConnectActionStatus(notebook: NotebookProcessedObject) {
    if (notebook.status.phase !== STATUS_TYPE.READY) {
      return STATUS_TYPE.UNAVAILABLE;
    }

    return STATUS_TYPE.READY;
  }

  public notebookTrackByFn(index: number, notebook: NotebookProcessedObject) {
    return `${notebook.name}/${notebook.image}`;
  }

  showAddPostDialog(notebook: NotebookProcessedObject) {

    this.currentName = notebook.name;
    this.dialog.open(AddPostDialogComponent, {
      hasBackdrop: false,
      data: { notebook: notebook}
    });

    //this.dialog.open(AddPostDialogComponent, {
    //  width: '600px',
    //  panelClass: 'form--dialog-padding',
    //});
  }
}
