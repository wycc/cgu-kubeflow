import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { IndexDefaultComponent } from './index-default.component';
import { AddPostConfirmDialogComponent } from './add-post-confirm-dialog/add-post-confirm-dialog.component';
import { AddPostDialogComponent } from './add-post-dialog/add-post-dialog.component';
import { AfterPostNotifyComponent } from './after-post-notify/after-post-notify.component';
import { KubeflowModule } from 'kubeflow';
import {MatDialogModule } from '@angular/material/dialog';

@NgModule({
  declarations: [IndexDefaultComponent, AddPostConfirmDialogComponent, AddPostDialogComponent, AfterPostNotifyComponent],
  imports: [CommonModule, KubeflowModule, MatDialogModule, FormsModule],
  exports: [IndexDefaultComponent],
})
export class IndexDefaultModule {}
