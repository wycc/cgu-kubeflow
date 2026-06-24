import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ReactiveFormsModule } from '@angular/forms';
import { OverviewComponent } from './overview.component';
import { MatButtonModule } from '@angular/material/button';
import { MatChipsModule } from '@angular/material/chips';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';
import {
  KubeflowModule,
  ConditionsTableModule,
  DetailsListModule,
  HeadingSubheadingRowModule,
  ContentListItemModule,
  VariablesGroupsTableModule,
  UrlsModule,
} from 'kubeflow';
import { ConfigurationsModule } from './configurations/configurations.module';
import { VolumesComponent } from './volumes/volumes.component';

@NgModule({
  declarations: [OverviewComponent, VolumesComponent],
  imports: [
    CommonModule,
    ReactiveFormsModule,
    DetailsListModule,
    ConditionsTableModule,
    HeadingSubheadingRowModule,
    KubeflowModule,
    ContentListItemModule,
    VariablesGroupsTableModule,
    ConfigurationsModule,
    UrlsModule,
    MatButtonModule,
    MatChipsModule,
    MatFormFieldModule,
    MatInputModule,
    MatSelectModule,
  ],
  exports: [OverviewComponent],
})
export class OverviewModule {}
