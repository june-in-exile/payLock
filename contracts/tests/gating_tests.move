#[test_only]
module paylock::gating_tests {
    use sui::test_scenario::{Self as ts, Scenario};
    use sui::coin;
    use sui::sui::SUI;
    use std::string;

    use paylock::gating::{Self, Video, AccessPass};

    const CREATOR: address = @0xCAFE;
    const BUYER: address = @0xBEEF;
    const PRICE: u64 = 100_000_000; // 0.1 SUI

    // Fixed 32-byte namespace for tests
    const TEST_NAMESPACE: vector<u8> = vector[
        0xAA, 0xBB, 0xCC, 0xDD, 0x01, 0x02, 0x03, 0x04,
        0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
        0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x12, 0x13, 0x14,
        0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x1C,
    ];

    fun create_test_video(scenario: &mut Scenario) {
        ts::next_tx(scenario, CREATOR);
        {
            let ctx = ts::ctx(scenario);
            gating::create_video(
                PRICE,
                string::utf8(b"preview_blob_abc"),
                string::utf8(b"full_blob_xyz"),
                TEST_NAMESPACE,
                ctx,
            );
        };
    }

    #[test]
    fun test_create_video() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        ts::next_tx(&mut scenario, CREATOR);
        {
            let video = ts::take_shared<Video>(&scenario);
            assert!(gating::video_price(&video) == PRICE);
            assert!(gating::video_creator(&video) == CREATOR);
            assert!(gating::video_preview_blob_id(&video) == &string::utf8(b"preview_blob_abc"));
            assert!(gating::video_full_blob_id(&video) == &string::utf8(b"full_blob_xyz"));
            let expected_ns = TEST_NAMESPACE;
            assert!(gating::video_seal_namespace(&video) == &expected_ns);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    fun test_create_free_video_empty_namespace() {
        let mut scenario = ts::begin(CREATOR);

        ts::next_tx(&mut scenario, CREATOR);
        {
            let ctx = ts::ctx(&mut scenario);
            gating::create_video(
                0,
                string::utf8(b"preview_blob_abc"),
                string::utf8(b"full_blob_xyz"),
                vector[],
                ctx,
            );
        };

        ts::next_tx(&mut scenario, CREATOR);
        {
            let video = ts::take_shared<Video>(&scenario);
            assert!(gating::video_price(&video) == 0);
            assert!(gating::video_seal_namespace(&video) == &vector[]);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    #[expected_failure(abort_code = gating::EMissingSealNamespace)]
    fun test_create_paid_video_empty_namespace_fails() {
        let mut scenario = ts::begin(CREATOR);

        ts::next_tx(&mut scenario, CREATOR);
        {
            let ctx = ts::ctx(&mut scenario);
            gating::create_video(
                PRICE,
                string::utf8(b"preview"),
                string::utf8(b"full"),
                vector[],
                ctx,
            );
        };

        ts::end(scenario);
    }

    #[test]
    fun test_purchase_exact_payment() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let mut payment = coin::mint_for_testing<SUI>(PRICE, ctx);
            let pass = gating::purchase(&video, &mut payment, ctx);

            assert!(gating::access_pass_video_id(&pass) == object::id(&video));
            assert!(coin::value(&payment) == 0);

            transfer::public_transfer(pass, BUYER);
            coin::destroy_zero(payment);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    fun test_purchase_overpayment() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let mut payment = coin::mint_for_testing<SUI>(PRICE * 2, ctx);
            let pass = gating::purchase(&video, &mut payment, ctx);

            assert!(coin::value(&payment) == PRICE);

            transfer::public_transfer(pass, BUYER);
            transfer::public_transfer(payment, BUYER);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    #[expected_failure(abort_code = gating::EInsufficientPayment)]
    fun test_purchase_insufficient_payment() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let mut payment = coin::mint_for_testing<SUI>(PRICE - 1, ctx);
            let pass = gating::purchase(&video, &mut payment, ctx);

            transfer::public_transfer(pass, BUYER);
            transfer::public_transfer(payment, BUYER);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    fun test_seal_approve_valid() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        // Purchase first
        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let mut payment = coin::mint_for_testing<SUI>(PRICE, ctx);
            let pass = gating::purchase(&video, &mut payment, ctx);
            transfer::public_transfer(pass, BUYER);
            coin::destroy_zero(payment);
            ts::return_shared(video);
        };

        // Seal approve using seal_namespace as prefix
        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let pass = ts::take_from_sender<AccessPass>(&scenario);

            // Build a valid seal ID: namespace bytes + nonce
            let mut seal_id = TEST_NAMESPACE;
            vector::push_back(&mut seal_id, 0x01);
            vector::push_back(&mut seal_id, 0x02);
            vector::push_back(&mut seal_id, 0x03);
            vector::push_back(&mut seal_id, 0x04);
            vector::push_back(&mut seal_id, 0x05);

            gating::seal_approve(seal_id, &pass, &video);

            ts::return_to_sender(&scenario, pass);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    #[expected_failure(abort_code = gating::EInvalidSealId)]
    fun test_seal_approve_wrong_namespace() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        // Purchase
        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let mut payment = coin::mint_for_testing<SUI>(PRICE, ctx);
            let pass = gating::purchase(&video, &mut payment, ctx);
            transfer::public_transfer(pass, BUYER);
            coin::destroy_zero(payment);
            ts::return_shared(video);
        };

        // Seal approve with wrong namespace — should fail
        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let pass = ts::take_from_sender<AccessPass>(&scenario);

            let wrong_namespace = vector[
                0xFF, 0xEE, 0xDD, 0xCC, 0x00, 0x00, 0x00, 0x00,
                0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
                0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
                0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
                0x01, 0x02, 0x03, 0x04, 0x05,
            ];

            gating::seal_approve(wrong_namespace, &pass, &video);

            ts::return_to_sender(&scenario, pass);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    #[expected_failure(abort_code = gating::EVideoMismatch)]
    fun test_seal_approve_wrong_video() {
        let mut scenario = ts::begin(CREATOR);

        // Create two videos
        create_test_video(&mut scenario);

        ts::next_tx(&mut scenario, CREATOR);
        {
            let ctx = ts::ctx(&mut scenario);
            gating::create_video(
                PRICE,
                string::utf8(b"other_preview"),
                string::utf8(b"other_full"),
                TEST_NAMESPACE,
                ctx,
            );
        };

        // Purchase first video
        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let mut payment = coin::mint_for_testing<SUI>(PRICE, ctx);
            let pass = gating::purchase(&video, &mut payment, ctx);
            transfer::public_transfer(pass, BUYER);
            coin::destroy_zero(payment);
            ts::return_shared(video);
        };

        // Try seal_approve with pass from video1 but referencing video2
        ts::next_tx(&mut scenario, BUYER);
        {
            let video1 = ts::take_shared<Video>(&scenario);
            let video2 = ts::take_shared<Video>(&scenario);
            let pass = ts::take_from_sender<AccessPass>(&scenario);

            let mut seal_id = TEST_NAMESPACE;
            vector::push_back(&mut seal_id, 0x01);
            // pass is for video1, but we pass video2 — should fail
            gating::seal_approve(seal_id, &pass, &video2);

            ts::return_to_sender(&scenario, pass);
            ts::return_shared(video1);
            ts::return_shared(video2);
        };

        ts::end(scenario);
    }

    #[test]
    fun test_purchase_and_transfer() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let payment = coin::mint_for_testing<SUI>(PRICE, ctx);
            gating::purchase_and_transfer(&video, payment, ctx);
            ts::return_shared(video);
        };

        // Verify buyer received AccessPass
        ts::next_tx(&mut scenario, BUYER);
        {
            let pass = ts::take_from_sender<AccessPass>(&scenario);
            let video = ts::take_shared<Video>(&scenario);
            assert!(gating::access_pass_video_id(&pass) == object::id(&video));
            ts::return_to_sender(&scenario, pass);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }
}
