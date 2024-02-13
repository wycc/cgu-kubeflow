import { Component, OnInit, OnDestroy } from '@angular/core';
import { FormGroup } from '@angular/forms';
import { Config, NotebookFormObject } from 'src/app/types';
import { Subscription } from 'rxjs';
import { Subject } from 'rxjs';
import {
  NamespaceService,
  BackendService,
  SnackBarService,
  SnackType,
  getNameError,
} from 'kubeflow';
import { Router } from '@angular/router';
import { getFormDefaults, initFormControls } from './utils';
import { JWABackendService } from 'src/app/services/backend.service';
import { environment } from '@app/environment';

// Lance - begin - 20230817
import { isEqual } from 'lodash';
import { NotebookResponseObject, NotebookProcessedObject } from 'src/app/types';
import { MatDialog } from '@angular/material/dialog';
import { AddPostDialogComponent } from '../../index/index-default/add-post-dialog/add-post-dialog.component';
// Lance - end - 20230817

import {
  defaultAdvancedConfig,
  defaultConfig,
  getDeleteDialogConfig,
  getStopDialogConfig,
} from '../../index/index-default/config';

@Component({
  selector: 'app-form-default',
  templateUrl: './form-default.component.html',
  styleUrls: ['./form-default.component.scss'],
})

export class FormDefaultComponent implements OnInit, OnDestroy {
  public isBasic = true;
  public isTeached = false;
  public applying$ = new Subject <boolean>();

  configAdvance = defaultAdvancedConfig;

  currNamespace = '';
  formCtrl: FormGroup;
  config: Config;

  ephemeral = false;
  defaultStorageclass = false;

  blockSubmit = false;
  formReady = false;
  existingNotebooks = new Set<string>();

  subscriptions = new Subscription();

  // Lance - begin - 20230817
  rawData: NotebookResponseObject[] = [];
  processedData: NotebookProcessedObject[] = [];
  // Lance - end - 20230817

  constructor(
    public namespaceService: NamespaceService,
    public backend: JWABackendService,
    public router: Router,
    public popup: SnackBarService,
    public dialog: MatDialog
  ) {}

  ngOnInit(): void {
    // Initialize the form control
    this.formCtrl = this.getFormDefaults();

    this.backend.getUsername().subscribe(username => {
      if (Object.keys(username).length === 0) {
        // Don't fire on empty config
        //console.log("NO username")
        this.isTeached = false;
        return;
      }

      if( username.substring(0,1) === "D" || username.substring(0,1) === "d" )
        this.isTeached = true;
      else
        this.isTeached = false;

      this.isTeached = true;

      // alert(username.substring(0,1));
      //console.log("username", username)
    });

    // Update the form Values from the default ones
    this.backend.getConfig().subscribe(config => {
      if (Object.keys(config).length === 0) {
        // Don't fire on empty config
        return;
      }

      this.config = config;
      this.initFormControls(this.formCtrl, config);
    });

    // Keep track of the selected namespace
    this.subscriptions.add(
      this.namespaceService.getSelectedNamespace().subscribe(namespace => {
        this.currNamespace = namespace;
        this.formCtrl.controls.namespace.setValue(this.currNamespace);

        // Lance - begin - 20230817
        if (this.currNamespace) {
          // this.backend.getNotebooks(this.currNamespace).subscribe(notebooks => {
            this.backend.getAllNotebooks(this.currNamespace).subscribe(notebooks => {
            if (!isEqual(this.rawData, notebooks)) {

              /*
              for ( let nb of notebooks) {
                if (nb.jsonStr === null) {
                  nb.isTemplate = 'no'
                }
              }
              */
             
              this.rawData = notebooks;

              // Update the frontend's state
              this.processedData = this.processIncomingData(notebooks);
              // this.poller.reset();
            }
          });
        }
        // Lance - end - 20230817
      }),
    );

    // Check if a default StorageClass is set
    this.backend.getDefaultStorageClass().subscribe(defaultClass => {
      if (defaultClass.length === 0) {
        this.defaultStorageclass = false;
        this.popup.open(
          $localize`No default Storage Class is set. Can't create new Disks for the new Notebook. Please use an Existing Disk.`,
          SnackType.Warning,
          0,
        );
      } else {
        this.defaultStorageclass = true;
      }
    });
  }

  ngOnDestroy() {
    // Unsubscriptions
    this.subscriptions.unsubscribe();
  }

  // Lance - begin - 20230817
  processIncomingData(notebooks: NotebookResponseObject[]) {
    const notebooksCopy = JSON.parse(
      JSON.stringify(notebooks),
    ) as NotebookProcessedObject[];

    for (const nb of notebooksCopy) {
      // this.updateNotebookFields(nb);
      if (nb.jsonStr === null) {
      }
    }
    return notebooksCopy;
  }
  // Lance - end - 20230817

  // Functions for handling the Form Group of the entire Form
  getFormDefaults() {
    return getFormDefaults();
  }

  initFormControls(formCtrl: FormGroup, config: Config) {
    initFormControls(formCtrl, config);
  }

  // Form Actions
  getSubmitNotebook(): NotebookFormObject {
    const notebookCopy = this.formCtrl.value as NotebookFormObject;
    const notebook = JSON.parse(JSON.stringify(notebookCopy));
    // console.log("notebookCopy", JSON.stringify(notebookCopy))
        // Use the custom image instead
    if (notebook.customImageCheck) {
      notebook.image = notebook.customImage;
    } else if (notebook.serverType === 'group-one') {
      // Set notebook image from imageGroupOne
      notebook.image = notebook.imageGroupOne;
    } else if (notebook.serverType === 'group-two') {
      // Set notebook image from imageGroupTwo
      notebook.image = notebook.imageGroupTwo;
    }

    // Remove unnecessary images from the request sent to the backend
    delete notebook.imageGroupOne;
    delete notebook.imageGroupTwo;

    // Ensure CPU input is a string
    if (typeof notebook.cpu === 'number') {
      notebook.cpu = notebook.cpu.toString();
    }

    // Ensure GPU input is a string
    if (typeof notebook.gpus.num === 'number') {
      notebook.gpus.num = notebook.gpus.num.toString();
    }

    // Remove cpuLimit from request if null
    if (notebook.cpuLimit == null) {
      delete notebook.cpuLimit;
      // Ensure CPU Limit input is a string
    } else if (typeof notebook.cpuLimit === 'number') {
      notebook.cpuLimit = notebook.cpuLimit.toString();
    }

    // Remove memoryLimit from request if null
    if (notebook.memoryLimit == null) {
      delete notebook.memoryLimit;
      // Add Gi to memoryLimit
    } else if (notebook.memoryLimit) {
      notebook.memoryLimit = notebook.memoryLimit.toString() + 'Gi';
    }

    // Add Gi to all sizes
    if (notebook.memory) {
      notebook.memory = notebook.memory.toString() + 'Gi';
    }

    for (const vol of notebook.datavols) {
      if (vol.size) {
        vol.size = vol.size + 'Gi';
      }
    }

    notebook.isTemplate = 'no';

    return notebook;
  }

  onSubmit() {
    this.popup.open('Submitting new Notebook...', SnackType.Info, 3000);

    const notebook = this.getSubmitNotebook();
    this.backend.createNotebook(notebook).subscribe(() => {
      this.popup.close();
      this.popup.open(
        'Notebook created successfully.',
        SnackType.Success,
        3000,
      );
      this.router.navigate(['/']);
    });
  }

  onCancel() {
    this.router.navigate(['/']);
  }

  onAdvanced() {
    this.isBasic = false;
    this.applying$.next(true);
  }

  onEdit(notebook: NotebookProcessedObject) {
    this.showAddPostDialog(notebook);
  }

  onCreateNotebook(notebookCopy) {
    // alert(notebook);
    const notebookFormCopy = this.formCtrl.value as NotebookFormObject;
    const notebookForm = JSON.parse(JSON.stringify(notebookFormCopy));

    const notebook = JSON.parse(notebookCopy);
    notebook.name = notebookForm.name;
    notebook.origin_namespace = notebook.namespace
    notebook.namespace = this.currNamespace
    notebook.template = notebook.workspace.newPvc.metadata.name
    notebook.workspace.newPvc.metadata.name = notebookForm.name + "-volume";
    this.backend.createNotebook(notebook).subscribe(() => {
      this.popup.close();
      this.popup.open(
        'Notebook created successfully.',
        SnackType.Success,
        3000,
      );
      this.router.navigate(['/']);
    });
  }

  showAddPostDialog(notebook: NotebookProcessedObject) {

    const ref = this.dialog.open(AddPostDialogComponent, {
      hasBackdrop: false,
      data: { notebook: notebook}
    });

    ref.afterClosed().subscribe(res => {
        window.location.reload();
        // alert("kkkkk");
    });

    
    //this.dialog.open(AddPostDialogComponent, {
    //  width: '600px',
    //  panelClass: 'form--dialog-padding',
    //});
  }
}
