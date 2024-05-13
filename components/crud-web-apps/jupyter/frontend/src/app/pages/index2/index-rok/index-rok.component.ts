import { Component, OnInit } from '@angular/core';
import { environment } from '@app/environment';
import {
  RokService,
  NamespaceService,
  SnackBarService,
  ConfirmDialogService,
} from 'kubeflow';
import { MatDialog } from '@angular/material/dialog';
import { JWABackendService } from 'src/app/services/backend.service';
import { ActivatedRoute, Router } from '@angular/router';
import { IndexDefaultComponent2 } from '../index-default2/index-default2.component';
import { HttpClient } from '@angular/common/http';
@Component({
  selector: 'app-index-rok',
  templateUrl: '../index-default2/index-default2.component.html',
  styleUrls: ['../index-default2/index-default2.component.scss'],
})
export class IndexRokComponent extends IndexDefaultComponent2 implements OnInit {
  constructor(
    private rok: RokService,
    public ns: NamespaceService,
    public backend: JWABackendService,
    public confirmDialog: ConfirmDialogService,
    public popup: SnackBarService,
    public router: Router,
    public dialog: MatDialog,
    public route: ActivatedRoute,
    public http: HttpClient
   
  ) {
    super(ns, backend, confirmDialog, popup, router, dialog, route,http);

    this.rok.initCSRF();
  }
}
