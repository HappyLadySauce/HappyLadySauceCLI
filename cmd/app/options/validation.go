package options

import "errors"

func (o *Options) Validate() error {
	var errs error
	errs = errors.Join(errs, o.NormalizeHome())
	errs = errors.Join(errs, o.Model.Validate())
	return errs
}
