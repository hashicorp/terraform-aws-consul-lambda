package validation

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/require"
)

func TestValidation_LambdaRegistrator(t *testing.T) {
	t.Parallel()
	terraformOpts := &terraform.Options{
		TerraformDir: "./terraform/registrator-config-validate",
		NoColor:      true,
	}
	terraform.Init(t, terraformOpts)
	cases := map[string]struct {
		tfVars    map[string]interface{}
		tfPlanErr string
	}{
		"ecr_image_uri and auto_publish not set": {
			tfVars: map[string]interface{}{
				"name":                          "test",
				"consul_http_addr":              "https://consul.example.com:8501",
				"ecr_image_uri":                 "",
				"enable_auto_publish_ecr_image": false,
			},
			tfPlanErr: "ERROR: either ecr_image_uri or enable_auto_publish_ecr_image must be set",
		},
		"ecr_image_uri set and auto_publish not set": {
			tfVars: map[string]interface{}{
				"name":                          "test",
				"consul_http_addr":              "https://consul.example.com:8501",
				"ecr_image_uri":                 "123456789012.dkr.ecr.us-east-1.amazonaws.com/my-ecr-repo:latest",
				"enable_auto_publish_ecr_image": false,
			},
			tfPlanErr: "",
		},
		"ecr_image_uri not set and auto_publish set": {
			tfVars: map[string]interface{}{
				"name":                          "test",
				"consul_http_addr":              "https://consul.example.com:8501",
				"ecr_image_uri":                 "",
				"enable_auto_publish_ecr_image": true,
			},
			tfPlanErr: "",
		},
		"ecr_image_uri set and auto_publish set": {
			tfVars: map[string]interface{}{
				"name":                          "test",
				"consul_http_addr":              "https://consul.example.com:8501",
				"ecr_image_uri":                 "123456789012.dkr.ecr.us-east-1.amazonaws.com/my-ecr-repo:latest",
				"enable_auto_publish_ecr_image": true,
			},
			tfPlanErr: "",
		},
	}
	for name, c := range cases {
		c := c
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := terraform.PlanE(t, &terraform.Options{
				TerraformDir: terraformOpts.TerraformDir,
				NoColor:      true,
				Vars:         c.tfVars,
			})
			if c.tfPlanErr != "" {
				require.Error(t, err)
				// error messages are wrapped, so a space may turn into a newline.
				regex := strings.ReplaceAll(regexp.QuoteMeta(c.tfPlanErr), " ", "\\s+")
				require.Regexp(t, regex, err.Error())
				return
			}
			require.NoError(t, err)
		})
	}
}
