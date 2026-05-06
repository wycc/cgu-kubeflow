import { ComponentFixture, TestBed } from '@angular/core/testing';
import {
  ContentListItemModule,
  DetailsListModule,
  HeadingSubheadingRowModule,
  LoadingSpinnerModule,
  PollerService,
  SnackBarModule,
  STATUS_TYPE,
  VariablesGroupsTableModule,
} from 'kubeflow';
import { JWABackendService } from 'src/app/services/backend.service';
import { ConfigurationsModule } from './configurations/configurations.module';
import { OverviewComponent } from './overview.component';
import { of } from 'rxjs';
import { mockNotebook } from '../notebook-mock';
import { mockPod } from '../pod-mock';
import { VolumesComponent } from './volumes/volumes.component';

const JWABackendServiceStub: Partial<JWABackendService> = {
  getPodDefaults: () => of([]),
  getNotebookNodeIp: () => of('10.10.10.10'),
  getNotebookSshNodePort: () => of(31234),
  getConfig: () => of({}),
};
const PollerServiceStub: Partial<PollerService> = {
  exponential: request => request,
};

describe('OverviewComponent', () => {
  let component: OverviewComponent;
  let fixture: ComponentFixture<OverviewComponent>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      declarations: [OverviewComponent, VolumesComponent],
      providers: [
        { provide: JWABackendService, useValue: JWABackendServiceStub },
        { provide: PollerService, useValue: PollerServiceStub },
      ],
      imports: [
        SnackBarModule,
        DetailsListModule,
        ContentListItemModule,
        ConfigurationsModule,
        VariablesGroupsTableModule,
        HeadingSubheadingRowModule,
        LoadingSpinnerModule,
      ],
    }).compileComponents();
  });

  beforeEach(() => {
    fixture = TestBed.createComponent(OverviewComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
    component.notebook = mockNotebook;
    component.pod = mockPod;
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('should return correct podDefaultsMessage', () => {
    let message = component.getPodDefaultsMessage(true, []);
    expect(message).toBeTruthy();

    message = component.getPodDefaultsMessage(true, [{}, {}]);
    expect(message).toBeFalsy();
  });

  it('should return correct docker image', () => {
    const dockerImage = component.getDockerImage(component.notebook);
    expect(dockerImage).toEqual(
      'public.ecr.aws/j1r0q0g6/notebooks/notebook-servers/rstudio-tidyverse:master-e9324d39',
    );
  });

  it('should load node port for the notebook', () => {
    expect(component.nodePort).toBe(31234);
  });

  it('should load node ip for the notebook', () => {
    expect(component.nodeIp).toBe('10.10.10.10');
  });

  it('should build the ssh command from node ip and node port', () => {
    expect(component.sshCommand).toBe('ssh -p 31234 jovyan@10.10.10.10');
  });

  it('should treat zero values as present display values', () => {
    expect(component.hasDisplayValue(0)).toBeTrue();
    expect(component.hasDisplayValue('0')).toBeTrue();
    expect(component.hasDisplayValue('0Gi')).toBeTrue();
    expect(component.hasDisplayValue(null)).toBeFalse();
    expect(component.hasDisplayValue('')).toBeFalse();
  });

  it('should display cpu units as cores', () => {
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('cores');
  });

  it('should format normalized cpu quantities for display', () => {
    const notebook = {
      ...component.notebook,
      spec: {
        ...component.notebook.spec,
        template: {
          ...component.notebook.spec.template,
          spec: {
            ...component.notebook.spec.template.spec,
            containers: component.notebook.spec.template.spec.containers.map(
              cn =>
                cn.name === component.notebook.metadata.name
                  ? {
                      ...cn,
                      resources: {
                        requests: {
                          ...(cn.resources.requests || {}),
                          cpu: '500m',
                        },
                        limits: {
                          ...(cn.resources.limits || {}),
                          cpu: '900m',
                        },
                      },
                    }
                  : cn,
            ),
          },
        },
      },
    };

    expect(component.getCpuRequest(notebook)).toBe('0.5');
    expect(component.getCpuLimits(notebook)).toBe('0.9');
  });

  it('should format normalized memory quantities for display', () => {
    const notebook = {
      ...component.notebook,
      spec: {
        ...component.notebook.spec,
        template: {
          ...component.notebook.spec.template,
          spec: {
            ...component.notebook.spec.template.spec,
            containers: component.notebook.spec.template.spec.containers.map(
              cn =>
                cn.name === component.notebook.metadata.name
                  ? {
                      ...cn,
                      resources: {
                        ...cn.resources,
                        limits: {
                          ...(cn.resources.limits || {}),
                          memory: '1288490188800m',
                        },
                      },
                    }
                  : cn,
            ),
          },
        },
      },
    };

    expect(component.getMemoryLimits(notebook)).toBe('1.2Gi');
  });

  it('should parse cpu and memory quantities into edit form values', () => {
    const notebook = {
      ...component.notebook,
      spec: {
        ...component.notebook.spec,
        template: {
          ...component.notebook.spec.template,
          spec: {
            ...component.notebook.spec.template.spec,
            containers: component.notebook.spec.template.spec.containers.map(
              cn =>
                cn.name === component.notebook.metadata.name
                  ? {
                      ...cn,
                      resources: {
                        requests: {
                          ...(cn.resources.requests || {}),
                          cpu: '500m',
                          memory: '1Gi',
                        },
                        limits: {
                          ...(cn.resources.limits || {}),
                          cpu: '900m',
                          memory: '1288490188800m',
                        },
                      },
                    }
                  : cn,
            ),
          },
        },
      },
    };

    component.notebook = notebook;

    expect(component.cpuControl.value).toBe(0.5);
    expect(component.cpuLimitControl.value).toBe(0.9);
    expect(component.memoryControl.value).toBe(1);
    expect(component.memoryLimitControl.value).toBeCloseTo(1.2, 5);
  });

  it('should keep zero cpu, memory, and gpu values visible', () => {
    const notebook = {
      ...component.notebook,
      spec: {
        ...component.notebook.spec,
        template: {
          ...component.notebook.spec.template,
          spec: {
            ...component.notebook.spec.template.spec,
            containers: component.notebook.spec.template.spec.containers.map(
              cn =>
                cn.name === component.notebook.metadata.name
                  ? {
                      ...cn,
                      resources: {
                        requests: {
                          ...(cn.resources.requests || {}),
                          cpu: '0',
                          memory: '0Gi',
                        },
                        limits: {
                          ...(cn.resources.limits || {}),
                          cpu: '0',
                          memory: '0Gi',
                          'nvidia.com/gpu': '0',
                        },
                      },
                    }
                  : cn,
            ),
          },
        },
      },
    };

    component.notebook = notebook;
    fixture.detectChanges();

    expect(component.cpuRequests).toBe('0');
    expect(component.cpuLimits).toBe('0');
    expect(component.memoryRequests).toBe('0Gi');
    expect(component.memoryLimits).toBe('0Gi');
    expect(component.gpuMessage).toBe('0 nvidia.com/gpu');
    expect(component.hasResourceValues).toBeTrue();
  });

  it('should keep the gpu row visible when gpu is set to none', () => {
    expect(component.gpuMessage).toBe('None');
    expect(component.hasDisplayValue(component.gpuMessage)).toBeTrue();
  });

  it('should expose validation helpers for invalid resource input', () => {
    component.cpuControl.setValue(0);
    component.cpuControl.markAsTouched();
    component.resourceForm.updateValueAndValidity();

    expect(component.showControlError(component.cpuControl, 'min')).toBeTrue();

    component.gpuNumControl.setValue('1');
    component.gpuVendorControl.setValue('');
    component.resourceForm.markAsTouched();
    component.resourceForm.updateValueAndValidity();

    expect(component.showFormError('gpuVendorRequired')).toBeTrue();
  });
});
