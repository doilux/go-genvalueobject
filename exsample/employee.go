package exsample

import "time"

// go:generate go-genvalueobject -Type=Employee,Department

type (

	Employee struct {
		id EmployeeID
		name string
		salary uint
		department *Department
		joinAt time.Time
	}
	
	Department struct {
		id int
		name string
	}

	EmployeeID int
)