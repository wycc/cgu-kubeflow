import { NgModule } from '@angular/core';
import { Routes, RouterModule } from '@angular/router';

import { IndexComponent } from './pages/index/index.component';
import { FormComponent } from './pages/form/form.component';
import { IndexComponent2 } from './pages/index2/index2.component';

const routes: Routes = [
  { path: '', component: IndexComponent },
  { path: 'new', component: FormComponent },
  { path: 'notebook/auto-start/:namespace/:notebook_name/view', component: IndexComponent2},
  { path: 'notebook/auto-start/:namespace/:notebook_name', component: IndexComponent2}
];

@NgModule({
  imports: [RouterModule.forRoot(routes, { relativeLinkResolution: 'legacy' })],
  exports: [RouterModule],
})
export class AppRoutingModule {}
