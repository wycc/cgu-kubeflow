import { Component, OnInit, OnDestroy } from '@angular/core';
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

import { MatDialog } from '@angular/material/dialog';
import { AddPostDialogComponent } from './add-post-dialog/add-post-dialog.component';

@Component({
  selector: 'app-index-default',
  templateUrl: './index-default.component.html',
  styleUrls: ['./index-default.component.scss'],
})
export class IndexDefaultComponent implements OnInit, OnDestroy {
  env = environment;
  poller: ExponentialBackoff;

  currNamespace = '';
  isBasic = false;
  configAdvance = defaultAdvancedConfig;
  config = defaultConfig;
  subs = new Subscription();

  currentName = '';

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

  ngOnInit(): void {
    this.poller = new ExponentialBackoff({ interval: 1000, retries: 3 });

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

      // alert(username.substring(0,1));
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
            this.processedData = this.processIncomingData(notebooks);
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
    if (notebook.status.phase === STATUS_TYPE.READY) {
      this.actions.connectToNotebookDetail(notebook.namespace, notebook.name);
    } else {
      this.actions
      .startNotebook(notebook.namespace, notebook.name)
      .subscribe(_ => {
        notebook.status.phase = STATUS_TYPE.WAITING;
        notebook.status.message = 'Starting the Notebook Server.';
        this.updateNotebookFields(notebook);
        this.actions.connectToNotebookDetail(notebook.namespace, notebook.name);
      });
    }
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
        this.shareClicked(a.data);
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
