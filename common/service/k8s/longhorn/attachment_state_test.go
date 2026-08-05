package longhorn

import (
	"testing"

	longhornV1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func TestVolumeAttachmentState(t *testing.T) {
	tests := []struct {
		name        string
		volumeState string
		withTicket  bool
		want        string
	}{
		{name: "attached with ticket", volumeState: "attached", withTicket: true, want: "attached"},
		{name: "attaching with ticket", volumeState: "attaching", withTicket: true, want: "attaching"},
		{name: "detached with ticket", volumeState: "detached", withTicket: true, want: "attaching"},
		{name: "attached without ticket", volumeState: "attached", want: "detaching"},
		{name: "attaching without ticket", volumeState: "attaching", want: "detaching"},
		{name: "detaching without ticket", volumeState: "detaching", want: "detaching"},
		{name: "detached without ticket", volumeState: "detached", want: "detached"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			volume := &longhornV1beta2.Volume{}
			volume.Name = "volume-1"
			volume.Status.State = longhornV1beta2.VolumeState(tt.volumeState)
			attachments := &longhornV1beta2.VolumeAttachmentList{}
			attachment := longhornV1beta2.VolumeAttachment{}
			attachment.Spec.Volume = volume.Name
			if tt.withTicket {
				attachment.Spec.AttachmentTickets = map[string]*longhornV1beta2.AttachmentTicket{
					"longhorn-ui": {ID: "longhorn-ui", NodeID: "server1"},
				}
			}
			attachments.Items = append(attachments.Items, attachment)

			if got := VolumeAttachmentState(volume, attachments); got != tt.want {
				t.Fatalf("VolumeAttachmentState() = %q, want %q", got, tt.want)
			}
		})
	}
}
