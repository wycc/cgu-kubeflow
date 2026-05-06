import { CommonModule } from '@angular/common';
import { ComponentFixture, TestBed, waitForAsync } from '@angular/core/testing';
import { RouterTestingModule } from '@angular/router/testing';
import {
  BackendService,
  ConfirmDialogService,
  KubeflowModule,
  NamespaceService,
  PollerService,
  SnackBarService,
  STATUS_TYPE,
} from 'kubeflow';
import { Observable, of } from 'rxjs';
import { MatDialog } from '@angular/material/dialog';
import { JWABackendService } from 'src/app/services/backend.service';
import { ActionsService } from 'src/app/services/actions.service';

import { IndexDefaultComponent } from './index-default.component';

const JWABackendServiceStub: Partial<JWABackendService> = {
  getNotebooks: () => of(),
  getManager: () => of(['user']),
  getUsername: () => of(''),
  deleteNotebook: () => of(),
  startNotebook: () => of(),
  stopNotebook: () => of(),
  setNotebookSsh: () => of('enabled'),
};

const NamespaceServiceStub: Partial<NamespaceService> = {
  getSelectedNamespace: () => of(),
  getSelectedNamespace2: () => of(),
};

const SnackBarServiceStub: Partial<SnackBarService> = {
  open: () => {},
  close: () => {},
};

const ActionsServiceStub: Partial<ActionsService> = {
  deleteNotebook: () => of('accept'),
  startNotebook: () => of('started'),
  stopNotebook: () => of('accept'),
  connectToNotebook: () => {},
};

const PollerServiceStub: Partial<PollerService> = {
  exponential: (obs: any) => obs,
};

const MatDialogStub: Partial<MatDialog> = {
  open: () =>
    ({
      afterClosed: () => of(null),
    } as any),
};

const MockBackendService: Partial<BackendService> = {
  getNamespaces(): Observable<string[]> {
    return of([]);
  },
};

describe('IndexDefaultComponent', () => {
  let component: IndexDefaultComponent;
  let fixture: ComponentFixture<IndexDefaultComponent>;

  beforeEach(waitForAsync(() => {
    TestBed.configureTestingModule({
      declarations: [IndexDefaultComponent],
      imports: [CommonModule, KubeflowModule, RouterTestingModule],
      providers: [
        { provide: JWABackendService, useValue: JWABackendServiceStub },
        { provide: NamespaceService, useValue: NamespaceServiceStub },
        { provide: SnackBarService, useValue: SnackBarServiceStub },
        { provide: PollerService, useValue: PollerServiceStub },
        { provide: ConfirmDialogService, useValue: {} },
        { provide: BackendService, useValue: MockBackendService },
        { provide: ActionsService, useValue: ActionsServiceStub },
        { provide: MatDialog, useValue: MatDialogStub },
      ],
    }).compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(IndexDefaultComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('returns READY for ssh action when ssh is enabled', () => {
    expect(
      component.processSshActionStatus({
        metadata: {
          labels: {
            'cgu.kubeflow.org/sshservice': 'true',
          },
        },
        status: {
          phase: STATUS_TYPE.READY,
        },
      } as any),
    ).toBe(STATUS_TYPE.READY);
  });

  it('returns UNINITIALIZED for ssh action when ssh is disabled', () => {
    expect(
      component.processSshActionStatus({
        metadata: {
          labels: {},
        },
        status: {
          phase: STATUS_TYPE.READY,
        },
      } as any),
    ).toBe(STATUS_TYPE.UNINITIALIZED);
  });

  it('returns WAITING for ssh action while toggle request is in progress', () => {
    expect(
      component.processSshActionStatus({
        metadata: {
          labels: {
            'cgu.kubeflow.org/sshservice': 'true',
          },
        },
        sshUpdating: true,
        status: {
          phase: STATUS_TYPE.READY,
        },
      } as any),
    ).toBe(STATUS_TYPE.WAITING);
  });
});
