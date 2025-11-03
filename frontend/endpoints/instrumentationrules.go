package endpoints

import (
	"github.com/gin-gonic/gin"
	odigosv1alpha1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/frontend/kube"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This type is used to hide the details of the k8s manifest and expose a generic object
type KarmaInstrumentationRule struct {
	odigosv1alpha1.KarmaInstrumentationRuleSpec

	// how the rule can be referenced in the REST api.
	// this is the name of the CR in k8s
	RuleId string `json:"ruleId"`
}

func GetKarmaInstrumentationRules(c *gin.Context, odigosns string) {

	instrumentationRules, err := kube.DefaultClient.CodekarmaClient.KarmaInstrumentationRules(odigosns).List(c, metav1.ListOptions{})
	if err != nil {
		c.JSON(500, gin.H{
			"message": "error getting instrumentation rules",
		})
		return
	}

	rules := make([]KarmaInstrumentationRule, 0, len(instrumentationRules.Items))
	for _, rule := range instrumentationRules.Items {
		rules = append(rules, KarmaInstrumentationRule{
			KarmaInstrumentationRuleSpec: rule.Spec,
			RuleId:                  rule.Name,
		})
	}

	// stringify the rules into json and return is
	c.JSON(200, rules)
}

func GetKarmaInstrumentationRule(c *gin.Context, odigosns string, ruleId string) {
	rule, err := kube.DefaultClient.CodekarmaClient.KarmaInstrumentationRules(odigosns).Get(c, ruleId, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.JSON(404, gin.H{
				"message": "instrumentation rule not found",
			})
			return
		}
		c.JSON(500, gin.H{
			"message": "error getting instrumentation rule",
		})
		return
	}

	c.JSON(200, KarmaInstrumentationRule{
		KarmaInstrumentationRuleSpec: rule.Spec,
		RuleId:                  rule.Name,
	})
}

func CreateKarmaInstrumentationRule(c *gin.Context, odigosns string) {
	var rule odigosv1alpha1.KarmaInstrumentationRuleSpec
	if err := c.BindJSON(&rule); err != nil {
		c.JSON(400, gin.H{
			"message": "invalid request body",
		})
		return
	}

	// create the rule
	createdRule, err := kube.DefaultClient.CodekarmaClient.KarmaInstrumentationRules(odigosns).Create(c, &odigosv1alpha1.KarmaInstrumentationRule{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "ui-instrumentation-rule-",
		},
		Spec: rule,
	}, metav1.CreateOptions{})
	if err != nil {
		c.JSON(500, gin.H{
			"message": "error creating instrumentation rule",
		})
		return
	}

	c.JSON(201, KarmaInstrumentationRule{
		KarmaInstrumentationRuleSpec: createdRule.Spec,
		RuleId:                  createdRule.Name,
	})
}

func DeleteKarmaInstrumentationRule(c *gin.Context, odigosns string, ruleId string) {
	err := kube.DefaultClient.CodekarmaClient.KarmaInstrumentationRules(odigosns).Delete(c, ruleId, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.JSON(404, gin.H{
				"message": "instrumentation rule not found",
			})
			return
		}
		c.JSON(500, gin.H{
			"message": "error deleting instrumentation rule",
		})
		return
	}

	c.JSON(204, nil)
}

func UpdateKarmaInstrumentationRule(c *gin.Context, odigosns string, ruleId string) {
	var rule odigosv1alpha1.KarmaInstrumentationRuleSpec
	if err := c.BindJSON(&rule); err != nil {
		c.JSON(400, gin.H{
			"message": "invalid request body",
		})
		return
	}

	// get existing rule
	existingRule, err := kube.DefaultClient.CodekarmaClient.KarmaInstrumentationRules(odigosns).Get(c, ruleId, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.JSON(404, gin.H{
				"message": "instrumentation rule not found",
			})
			return
		}
		c.JSON(500, gin.H{
			"message": err.Error(),
		})
		return
	}

	existingRule.Spec = rule

	// update the rule
	updatedRule, err := kube.DefaultClient.CodekarmaClient.KarmaInstrumentationRules(odigosns).Update(c, existingRule, metav1.UpdateOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.JSON(404, gin.H{
				"message": "instrumentation rule not found",
			})
			return
		}
		c.JSON(500, gin.H{
			"message": err.Error(),
		})
		return
	}

	c.JSON(200, KarmaInstrumentationRule{
		KarmaInstrumentationRuleSpec: updatedRule.Spec,
		RuleId:                  updatedRule.Name,
	})
}
